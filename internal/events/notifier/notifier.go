package notifier

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/cartesi/rollups-node/internal/events"
	"github.com/cartesi/rollups-node/pkg/service"
)

const anyApplicationID = events.ApplicationID("")

var EventTypeAll = []events.EventType{
	events.EventAppRegistered,
	events.EventAppDeactivated,
	events.EventAppReactivated,
	events.EventAppInoperable,
	events.EventEpochOpened,
	events.EventEpochClosed,
	events.EventEpochInputsProcessed,
	events.EventEpochClaimCalculated,
	events.EventEpochClaimSubmitted,
	events.EventEpochClaimAccepted,
	events.EventEpochClaimRejected,
	events.EventInputReceived,
}

type Subscription struct {
	notifyFunc   events.NotifyFunc
	notifier     *Notifier
	filter       events.SubscriptionFilter
	unlistenOnce sync.Once
}

func (s *Subscription) Unlisten(ctx context.Context) {
	s.unlistenOnce.Do(func() {
		// Unlisten strips cancellation from the parent context to ensure it runs:
		if err := s.notifier.unlisten(context.WithoutCancel(ctx), s); err != nil {
			s.notifier.Logger.ErrorContext(ctx, s.notifier.Name+": Error unlistening on topic", "err", err, "filter", s.filter)
		}
	})
}

type SubscriptionList []*Subscription
type ApplicationToSubscriptionsMap map[events.ApplicationID]SubscriptionList
type EventAppSubscriptionIndex map[events.EventType]ApplicationToSubscriptionsMap

type Notifier struct {
	// Logger is a structured logger.
	Logger *slog.Logger

	// Name is a name of the service. It should generally be used to prefix all
	// log lines the service emits.
	Name string

	BaseStartStop

	listener          events.Listener
	notificationBuf   chan *events.Notification
	waitInterruptChan chan func()

	mu            sync.RWMutex
	isConnected   bool
	isStarted     bool
	isWaiting     bool
	subscriptions EventAppSubscriptionIndex
	waitCancel    context.CancelFunc
}

func NewNotifier(listener events.Listener) *Notifier {
	return &Notifier{
		// TODO: review this values. Should come from a config.
		Logger: service.NewLogger(slog.LevelDebug, true),
		Name:   "PostgreSQL Notifier",

		listener:          listener,
		notificationBuf:   make(chan *events.Notification, 1000),
		waitInterruptChan: make(chan func(), 10),

		subscriptions: make(EventAppSubscriptionIndex),
	}
}

func (n *Notifier) Start(ctx context.Context) error {
	ctx, shouldStart, started, stopped := n.StartInit(ctx)
	if !shouldStart {
		return nil
	}

	// The loop below will connect/close on every iteration, but do one initial
	// connect so the notifier fails fast in case of an obvious problem.
	if err := n.listenerConnect(ctx, false); err != nil {
		stopped()
		if errors.Is(err, context.Canceled) {
			return nil
		}
		return err
	}

	go func() {
		started()
		defer stopped()

		n.Logger.DebugContext(ctx, n.Name+": Run loop started")
		defer n.Logger.DebugContext(ctx, n.Name+": Run loop stopped")

		n.withLock(func() { n.isStarted = true })
		defer n.withLock(func() { n.isStarted = false })

		defer n.listenerClose(ctx, false)

		var wg sync.WaitGroup

		wg.Go(func() {
			n.deliverNotifications(ctx)
		})

		for attempt := 0; ; attempt++ {
			if err := n.listenAndWait(ctx); err != nil {
				if errors.Is(err, context.Canceled) {
					break
				}

				sleepDuration := ExponentialBackoff(attempt, MaxAttemptsBeforeResetDefault)
				n.Logger.ErrorContext(ctx, n.Name+": Error running listener (will attempt reconnect after backoff)",
					slog.Int("attempt", attempt),
					slog.String("err", err.Error()),
					slog.String("sleep_duration", sleepDuration.String()),
				)
				CancellableSleep(ctx, sleepDuration)
			}
		}

		wg.Wait()
	}()

	return nil
}

func (n *Notifier) deliverNotifications(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return

		case notification := <-n.notificationBuf:
			evtType := events.EventType(notification.Topic)
			var event events.Event
			err := json.Unmarshal([]byte(notification.Payload), &event)
			if err != nil {
				n.Logger.ErrorContext(ctx, "Unmarshal error of notification", "error", err)
			} else {
				notifyFuncs := func() []events.NotifyFunc {
					n.mu.RLock()
					defer n.mu.RUnlock()

					appSubIdx := n.subscriptions[evtType]
					oneAppSubs := appSubIdx[event.AppID]
					anyAppSubs := appSubIdx[anyApplicationID]

					funcs := make([]events.NotifyFunc, len(oneAppSubs)+len(anyAppSubs))
					i := 0
					for _, subs := range []SubscriptionList{oneAppSubs, anyAppSubs} {
						for _, sub := range subs {
							funcs[i] = sub.notifyFunc
							i++
						}
					}
					return funcs
				}()

				for _, notifyFunc := range notifyFuncs {
					// TODO: panic recovery on delivery attempts
					notifyFunc(event)
				}
			}
		}
	}
}

func (n *Notifier) listenAndWait(ctx context.Context) error {
	if err := n.listenerConnect(ctx, false); err != nil {
		return err
	}
	defer n.listenerClose(ctx, false)

	topics := func() []string {
		n.mu.RLock()
		defer n.mu.RUnlock()

		topics := make([]string, 0, len(n.subscriptions))
		for evtType := range n.subscriptions {
			topics = append(topics, string(evtType))
		}
		return topics
	}()

	if err := n.listenerListen(ctx, topics); err != nil {
		return err
	}

	n.Logger.DebugContext(ctx, n.Name+": Notifier healthy")

	drainInterrupts := func() {
		for {
			select {
			case interruptOperation := <-n.waitInterruptChan:
				interruptOperation()
			default:
				return
			}
		}
	}

	// Drain interrupts one last time before leaving to make sure we're not
	// leaving any goroutines hanging anywhere.
	defer drainInterrupts()

	for {
		// Top level context is done, meaning we're shutting down.
		if ctx.Err() != nil {
			return ctx.Err()
		}

		// Drain any and all interrupt operations before continuing back into a
		// new wait to give any new subscribers a chance to listen/unlisten.
		drainInterrupts()

		err := n.waitOnce(ctx)
		if err != nil {
			// On cancellation, reenter loop, but the check at the top on
			// `ctx.Err()` will end it if the service is shutting down.
			if errors.Is(err, context.Canceled) {
				continue
			}

			n.Logger.InfoContext(ctx, n.Name+": Notifier unhealthy")

			return err
		}
	}
}

func (n *Notifier) listenerClose(ctx context.Context, skipLock bool) {
	if !skipLock {
		n.mu.Lock()
		defer n.mu.Unlock()
	}

	if !n.isConnected {
		return
	}

	n.Logger.DebugContext(ctx, n.Name+": Listener closing")
	if err := n.listener.Close(ctx); err != nil {
		if !errors.Is(err, context.Canceled) {
			n.Logger.ErrorContext(ctx, n.Name+": Error closing listener", "err", err)
		}
	}

	n.isConnected = false
}

const listenerTimeout = 10 * time.Second

func (n *Notifier) listenerConnect(ctx context.Context, skipLock bool) error {
	if !skipLock {
		n.mu.Lock()
		defer n.mu.Unlock()
	}

	if n.isConnected {
		return nil
	}

	ctx, cancel := context.WithTimeout(ctx, listenerTimeout)
	defer cancel()

	n.Logger.DebugContext(ctx, n.Name+": Listener connecting")
	if err := n.listener.Connect(ctx); err != nil {
		if !errors.Is(err, context.Canceled) {
			n.Logger.ErrorContext(ctx, n.Name+": Error connecting listener", "err", err)
		}

		return err
	}

	n.isConnected = true
	return nil
}

// Listens on a topic with an appropriate logging statement. Should be preferred
// to `listener.Listen` for improved logging/telemetry.
//
// Not protected by mutex because it doesn't modify any notifier state and the
// underlying listener has a mutex around its operations.
func (n *Notifier) listenerListen(ctx context.Context, topics []string) error {
	ctx, cancel := context.WithTimeout(ctx, listenerTimeout)
	defer cancel()

	n.Logger.DebugContext(ctx, n.Name+": Listening on topic", "topics", topics)
	if err := n.listener.Listen(ctx, topics); err != nil {
		return fmt.Errorf("error listening on topics %q: %w", topics, err)
	}

	return nil
}

// Unlistens on a topic with an appropriate logging statement. Should be
// preferred to `listener.Unlisten` for improved logging/telemetry.
//
// Not protected by mutex because it doesn't modify any notifier state and the
// underlying listener has a mutex around its operations.
func (n *Notifier) listenerUnlisten(ctx context.Context, topics []string) error {
	ctx, cancel := context.WithTimeout(ctx, listenerTimeout)
	defer cancel()

	n.Logger.DebugContext(ctx, n.Name+": Unlistening on topic", "topic", topics)
	if err := n.listener.Unlisten(ctx, topics); err != nil {
		return fmt.Errorf("error unlistening on topics %q: %w", topics, err)
	}

	return nil
}

const pingInterval = 5 * time.Second

// Enters a single blocking wait for notifications on the underlying listener.
// Waiting for a notification locks an underlying connection, so infrastructure
// elsewhere in the notifier must preempt it by sending to `n.waitInterruptChan`
// and invoking `n.waitCancel()`. Cancelling the input context (as occurs during
// shutdown) also unblocks the wait.
func (n *Notifier) waitOnce(ctx context.Context) error {
	n.withLock(func() {
		n.isWaiting = true
		ctx, n.waitCancel = context.WithCancel(ctx) //nolint:fatcontext
	})
	defer n.withLock(func() {
		n.isWaiting = false
		n.waitCancel()
	})

	// Save a reference to the parent context before creating the inner
	// cancellable context. The inner context is cancelled by drainErrChan to
	// interrupt WaitForNotification, but we still need a live context for the
	// Ping health check afterward.
	pingCtx := ctx

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	errChan := make(chan error)

	go func() {
		for {
			notification, err := n.listener.WaitForNotification(ctx)
			if err != nil {
				errChan <- err
				return
			}

			select {
			case n.notificationBuf <- notification:
			default:
				n.Logger.WarnContext(ctx, n.Name+": Dropping notification due to full buffer", "payload", notification.Payload)
			}
		}
	}()

	drainErrChan := func() error {
		cancel()

		// There's a chance we encounter some other error before the context.Canceled comes in:
		err := <-errChan
		if err != nil && !errors.Is(err, context.Canceled) {
			// A non-cancel error means something went wrong with the conn, so we should bail.
			n.Logger.ErrorContext(ctx, n.Name+": Error on draining notification wait", "err", err)
			return err
		}
		// If we got a context cancellation error, it means we successfully
		// interrupted the WaitForNotification so that we could make the
		// subscription change.
		return nil
	}

	needPingCtx, needPingCancel := context.WithTimeout(ctx, pingInterval)
	defer needPingCancel()

	// * Wait for notifications
	// * Ping conn if 5 seconds have elapsed between notifications to keep it alive
	// * Manage listens/unlistens on conn (waitInterruptChan)
	// * If any errors are encountered, return them so we can kill the conn and start over
	select {
	case <-ctx.Done():
		return <-errChan

	case <-needPingCtx.Done():
		if err := drainErrChan(); err != nil {
			return err
		}
		// Ping the conn to see if it's still alive. Use pingCtx (the parent
		// context) because the inner ctx was cancelled by drainErrChan above
		// to interrupt WaitForNotification.
		//
		// Note: Previously this used the (already cancelled) inner ctx, making
		// the ping a no-op that always returned context.Canceled. With the fix,
		// dead or flaky connections are now actively detected, which may trigger
		// reconnections that were previously silently swallowed.
		if err := n.listener.Ping(pingCtx); err != nil {
			return err
		}

	case err := <-errChan:
		if errors.Is(err, context.Canceled) {
			return nil
		}
		if err != nil {
			n.Logger.ErrorContext(ctx, n.Name+": Error from notification wait", "err", err)
			return err
		}
	}

	return nil
}

const interruptTimeout = 5 * time.Second

// Sends an interrupt operation to the main loop, waits on the result, and
// returns an error if there was one.
//
// MUST be called with the `n.mu` mutex already locked.
func (n *Notifier) sendInterruptAndReceiveResult(operation func() error) error {
	errChan := make(chan error)
	n.waitInterruptChan <- func() {
		errChan <- operation()
	}

	n.waitCancel()

	// Notably, these unlock then lock again, the reverse of what you'd normally
	// expect in a mutex pattern. This is because this function is only expected
	// to be called with the mutex already locked, but we need to unlock it to
	// give the main loop a chance to run interrupt operations.
	n.mu.Unlock()
	defer n.mu.Lock()

	select {
	case err := <-errChan:
		return err
	case <-time.After(interruptTimeout):
		return errors.New("timed out waiting for interrupt operation")
	}
}

func (n *Notifier) Listen(ctx context.Context, filter events.SubscriptionFilter, notifyFunc events.NotifyFunc) (*Subscription, error) {
	n.mu.Lock()
	defer n.mu.Unlock()

	if len(filter.EventTypes) == 0 {
		filter.EventTypes = EventTypeAll
	}
	if len(filter.AppIDs) == 0 {
		filter.AppIDs = []events.ApplicationID{anyApplicationID}
	}

	sub := &Subscription{
		notifyFunc: notifyFunc,
		filter:     filter,
		notifier:   n,
	}

	newTopics := make([]string, 0, len(filter.EventTypes))
	for _, evtType := range filter.EventTypes {
		appSubIdx, existingEvtType := n.subscriptions[evtType]
		if !existingEvtType {
			newTopics = append(newTopics, string(evtType))
			appSubIdx = make(ApplicationToSubscriptionsMap)
			n.subscriptions[evtType] = appSubIdx
		}
		for _, appID := range filter.AppIDs {
			subList, existingAppID := appSubIdx[appID]
			if !existingAppID {
				subList = make(SubscriptionList, 0, 10)
			}
			appSubIdx[appID] = append(subList, sub)
		}
	}

	n.Logger.DebugContext(ctx, n.Name+": Added subscription")

	// We add the new subscription to the subscription list optimistically, and
	// it needs to be done this way in case of a restart after an interrupt
	// below has been run, but after a return to this function (say we were to
	// add the new sub at the end of this function, it would not be picked
	// during the restart). But in case of an error subscribing, remove the sub.
	//
	// By the time this function is run (i.e. after an interrupt), a lock on
	// `n.mu` has been reacquired, and modifying subscription state is safe.
	removeSub := func() { n.removeSubscription(ctx, sub) }

	if len(newTopics) > 0 {
		// If already waiting, send an interrupt to the wait function to run a
		// listen operation. If not, connect and listen directly, returning any
		// errors as feedback to the caller.
		if n.isWaiting {
			if err := n.sendInterruptAndReceiveResult(func() error { return n.listenerListen(ctx, newTopics) }); err != nil {
				removeSub()
				return nil, err
			}
		} else {
			var justConnected bool

			if !n.isConnected {
				if err := n.listenerConnect(ctx, true); err != nil {
					removeSub()
					return nil, err
				}
				justConnected = true
			}

			if err := n.listenerListen(ctx, newTopics); err != nil {
				removeSub()

				// If we just connected above and the notifier hasn't started in
				// the interim, also close the connection so we don't leave any
				// resources hanging.
				if justConnected && !n.isStarted {
					n.listenerClose(ctx, true)
				}

				return nil, err
			}
		}
	}

	return sub, nil
}

func (n *Notifier) unlisten(ctx context.Context, sub *Subscription) error {
	n.mu.Lock()
	defer n.mu.Unlock()

	dismissedTopics := make([]string, 0, len(n.subscriptions))
	for _, evtType := range sub.filter.EventTypes {
		appSubIdx := n.subscriptions[evtType]
		dismissedAppIDCount := 0
		for _, appID := range sub.filter.AppIDs {
			if len(appSubIdx[appID]) == 1 {
				dismissedAppIDCount++
			}
		}
		if len(appSubIdx) == dismissedAppIDCount {
			dismissedTopics = append(dismissedTopics, string(evtType))
		}
	}

	// If there are topics to be dismissed, unlisten if we're connected.
	if len(dismissedTopics) > 0 {
		// If already waiting, send an interrupt to the wait function to run an
		// unlisten operation. If not, if connected, unlisten directly.
		if n.isWaiting {
			if err := n.sendInterruptAndReceiveResult(func() error { return n.listenerUnlisten(ctx, dismissedTopics) }); err != nil {
				return err
			}
		} else {
			if n.isConnected {
				if err := n.listenerUnlisten(ctx, dismissedTopics); err != nil {
					return err
				}

				// If this was the last subscription, we weren't in a wait loop,
				// and the notifier never started, also clean up by closing the
				// listener.
				if !n.isStarted && len(n.subscriptions) <= 1 {
					n.listenerClose(ctx, true)
				}
			}
		}
	}

	n.removeSubscription(ctx, sub)

	return nil
}

// This function requires that the caller already has a lock on `n.mu`.
func (n *Notifier) removeSubscription(ctx context.Context, sub *Subscription) {
	for _, evtType := range sub.filter.EventTypes {
		appSubIdx := n.subscriptions[evtType]
		for _, appID := range sub.filter.AppIDs {
			subList := appSubIdx[appID]
			subCount := len(subList)
			if subCount > 1 {
				for i, currSub := range subList {
					if currSub == sub {
						subCount--
						subList[i] = subList[subCount]
						appSubIdx[appID] = subList[:subCount]
					}
				}
			} else {
				delete(appSubIdx, appID)
			}
		}
		if len(appSubIdx) == 0 {
			delete(n.subscriptions, evtType)
		}
	}

	n.Logger.DebugContext(
		ctx,
		n.Name+": Removed subscription",
	)
}

func (n *Notifier) withLock(lockedFunc func()) {
	n.mu.Lock()
	defer n.mu.Unlock()
	lockedFunc()
}
