package events

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/cartesi/rollups-node/pkg/service"
)

const anyApplicationID = ApplicationID("")

var eventTypeAll = []EventType{
	EventAppRegistered,
	EventAppDeactivated,
	EventAppReactivated,
	EventAppInoperable,
	EventEpochOpened,
	EventEpochClosed,
	EventEpochInputsProcessed,
	EventEpochClaimCalculated,
	EventEpochClaimSubmitted,
	EventEpochClaimAccepted,
	EventEpochClaimRejected,
	EventInputReceived,
}

type eventSubscription struct {
	channel      chan Event
	service      *eventService
	filter       SubscriptionFilter
	unlistenOnce sync.Once
}

func (s *eventSubscription) Close(ctx context.Context) error {
	s.unlistenOnce.Do(func() {
		// Unlisten strips cancellation from the parent context to ensure it runs:
		if err := s.service.unlisten(context.WithoutCancel(ctx), s); err != nil {
			s.service.Logger.ErrorContext(ctx, s.service.Name+": Error unlistening on topic", "err", err, "filter", s.filter)
		}
		s.service = nil
		close(s.channel)
	})
	return nil
}

func (s *eventSubscription) Channel() <-chan Event {
	return s.channel
}

type subscriptionList []*eventSubscription
type applicationToSubscriptionsMap map[ApplicationID]subscriptionList
type eventAppSubscriptionIndex map[EventType]applicationToSubscriptionsMap

type eventService struct {
	// Logger is a structured logger.
	Logger *slog.Logger

	// Name is a name of the service. It should generally be used to prefix all
	// log lines the service emits.
	Name string

	BaseStartStop

	driver            Driver
	notificationBuf   chan *Notification
	waitInterruptChan chan func()

	mu            sync.RWMutex
	isConnected   bool
	isStarted     bool
	isWaiting     bool
	subscriptions eventAppSubscriptionIndex
	waitCancel    context.CancelFunc
}

func NewEventsService(driver Driver) Service {
	return &eventService{
		// TODO: review this values. Should come from a config.
		Logger: service.NewLogger(slog.LevelDebug, true),
		Name:   "Subscriber",

		driver:            driver,
		notificationBuf:   make(chan *Notification, 1000),
		waitInterruptChan: make(chan func(), 10),

		subscriptions: make(eventAppSubscriptionIndex),
	}
}

func (s *eventService) Start(ctx context.Context) error {
	ctx, shouldStart, started, stopped := s.StartInit(ctx)
	if !shouldStart {
		return nil
	}

	// The loop below will connect/close on every iteration, but do one initial
	// connect so the notifier fails fast in case of an obvious problem.
	if err := s.driverConnect(ctx, false); err != nil {
		stopped()
		if errors.Is(err, context.Canceled) {
			return nil
		}
		return err
	}

	go func() {
		started()
		defer stopped()

		s.Logger.DebugContext(ctx, s.Name+": Run loop started")
		defer s.Logger.DebugContext(ctx, s.Name+": Run loop stopped")

		s.withLock(func() { s.isStarted = true })
		defer s.withLock(func() { s.isStarted = false })

		defer s.driverClose(ctx, false)

		var wg sync.WaitGroup

		wg.Go(func() {
			s.deliverNotifications(ctx)
		})

		for attempt := 0; ; attempt++ {
			if err := s.listenAndWait(ctx); err != nil {
				if errors.Is(err, context.Canceled) {
					break
				}

				sleepDuration := ExponentialBackoff(attempt, MaxAttemptsBeforeResetDefault)
				s.Logger.ErrorContext(ctx, s.Name+": Error running driver (will attempt reconnect after backoff)",
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

func (s *eventService) deliverNotifications(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return

		case notification := <-s.notificationBuf:
			evtType := EventType(notification.Topic)
			var event Event
			err := json.Unmarshal([]byte(notification.Payload), &event)
			if err != nil {
				s.Logger.ErrorContext(ctx, "Unmarshal error of notification", "error", err)
			} else {
				channels := func() []chan Event {
					s.mu.RLock()
					defer s.mu.RUnlock()

					appSubIdx := s.subscriptions[evtType]
					oneAppSubs := appSubIdx[event.AppID]
					anyAppSubs := appSubIdx[anyApplicationID]

					channels := make([]chan Event, len(oneAppSubs)+len(anyAppSubs))
					i := 0
					for _, subs := range []subscriptionList{oneAppSubs, anyAppSubs} {
						for _, sub := range subs {
							channels[i] = sub.channel
							i++
						}
					}
					return channels
				}()

				for _, channel := range channels {
					select {
						case channel <- event:
						default:
							s.Logger.DebugContext(
								ctx,
								"subscription channel is full, dropping event",
								"EventType", event.Type,
								"AppID", event.AppID,
							)
					}
				}
			}
		}
	}
}

func (s *eventService) listenAndWait(ctx context.Context) error {
	if err := s.driverConnect(ctx, false); err != nil {
		return err
	}
	defer s.driverClose(ctx, false)

	topics := func() []string {
		s.mu.RLock()
		defer s.mu.RUnlock()

		topics := make([]string, 0, len(s.subscriptions))
		for evtType := range s.subscriptions {
			topics = append(topics, string(evtType))
		}
		return topics
	}()

	if err := s.driverListen(ctx, topics); err != nil {
		return err
	}

	s.Logger.DebugContext(ctx, s.Name+": Subscriber healthy")

	drainInterrupts := func() {
		for {
			select {
			case interruptOperation := <-s.waitInterruptChan:
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

		err := s.waitOnce(ctx)
		if err != nil {
			// On cancellation, reenter loop, but the check at the top on
			// `ctx.Err()` will end it if the service is shutting down.
			if errors.Is(err, context.Canceled) {
				continue
			}

			s.Logger.InfoContext(ctx, s.Name+": Subscriber unhealthy")

			return err
		}
	}
}

func (s *eventService) driverClose(ctx context.Context, skipLock bool) {
	if !skipLock {
		s.mu.Lock()
		defer s.mu.Unlock()
	}

	if !s.isConnected {
		return
	}

	s.Logger.DebugContext(ctx, s.Name+": Driver closing")
	if err := s.driver.Close(ctx); err != nil {
		if !errors.Is(err, context.Canceled) {
			s.Logger.ErrorContext(ctx, s.Name+": Error closing driver", "err", err)
		}
	}

	s.isConnected = false
}

const listenerTimeout = 10 * time.Second

func (s *eventService) driverConnect(ctx context.Context, skipLock bool) error {
	if !skipLock {
		s.mu.Lock()
		defer s.mu.Unlock()
	}

	if s.isConnected {
		return nil
	}

	ctx, cancel := context.WithTimeout(ctx, listenerTimeout)
	defer cancel()

	s.Logger.DebugContext(ctx, s.Name+": Driver connecting")
	if err := s.driver.Connect(ctx); err != nil {
		if !errors.Is(err, context.Canceled) {
			s.Logger.ErrorContext(ctx, s.Name+": Error connecting driver", "err", err)
		}

		return err
	}

	s.isConnected = true
	return nil
}

// Listens on a topic with an appropriate logging statement.
//
// Not protected by mutex because it doesn't modify any notifier state and the
// underlying driver has a mutex around its operations.
func (s *eventService) driverListen(ctx context.Context, topics []string) error {
	ctx, cancel := context.WithTimeout(ctx, listenerTimeout)
	defer cancel()

	s.Logger.DebugContext(ctx, s.Name+": Listening on topic", "topics", topics)
	if err := s.driver.Listen(ctx, topics); err != nil {
		return fmt.Errorf("error listening on topics %q: %w", topics, err)
	}

	return nil
}

// Unlistens on a topic with an appropriate logging statement.
//
// Not protected by mutex because it doesn't modify any service state and the
// underlying drvier has a mutex around its operations.
func (s *eventService) driverUnlisten(ctx context.Context, topics []string) error {
	ctx, cancel := context.WithTimeout(ctx, listenerTimeout)
	defer cancel()

	s.Logger.DebugContext(ctx, s.Name+": Unlistening on topic", "topic", topics)
	if err := s.driver.Unlisten(ctx, topics); err != nil {
		return fmt.Errorf("error unlistening on topics %q: %w", topics, err)
	}

	return nil
}

const pingInterval = 5 * time.Second

// Enters a single blocking wait for notifications on the underlying driver.
// Waiting for a notification locks an underlying connection, so infrastructure
// elsewhere in the service must preempt it by sending to `n.waitInterruptChan`
// and invoking `n.waitCancel()`. Cancelling the input context (as occurs during
// shutdown) also unblocks the wait.
func (s *eventService) waitOnce(ctx context.Context) error {
	s.withLock(func() {
		s.isWaiting = true
		ctx, s.waitCancel = context.WithCancel(ctx) //nolint:fatcontext
	})
	defer s.withLock(func() {
		s.isWaiting = false
		s.waitCancel()
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
			notification, err := s.driver.WaitForNotification(ctx)
			if err != nil {
				errChan <- err
				return
			}

			select {
			case s.notificationBuf <- notification:
			default:
				s.Logger.WarnContext(ctx, s.Name+": Dropping notification due to full buffer", "payload", notification.Payload)
			}
		}
	}()

	drainErrChan := func() error {
		cancel()

		// There's a chance we encounter some other error before the context.Canceled comes in:
		err := <-errChan
		if err != nil && !errors.Is(err, context.Canceled) {
			// A non-cancel error means something went wrong with the conn, so we should bail.
			s.Logger.ErrorContext(ctx, s.Name+": Error on draining notification wait", "err", err)
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
		if err := s.driver.Ping(pingCtx); err != nil {
			return err
		}

	case err := <-errChan:
		if errors.Is(err, context.Canceled) {
			return nil
		}
		if err != nil {
			s.Logger.ErrorContext(ctx, s.Name+": Error from notification wait", "err", err)
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
func (s *eventService) sendInterruptAndReceiveResult(operation func() error) error {
	errChan := make(chan error)
	s.waitInterruptChan <- func() {
		errChan <- operation()
	}

	s.waitCancel()

	// Notably, these unlock then lock again, the reverse of what you'd normally
	// expect in a mutex pattern. This is because this function is only expected
	// to be called with the mutex already locked, but we need to unlock it to
	// give the main loop a chance to run interrupt operations.
	s.mu.Unlock()
	defer s.mu.Lock()

	select {
	case err := <-errChan:
		return err
	case <-time.After(interruptTimeout):
		return errors.New("timed out waiting for interrupt operation")
	}
}

func (s *eventService) Subscribe(ctx context.Context, filter SubscriptionFilter) (Subscription, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(filter.EventTypes) == 0 {
		filter.EventTypes = eventTypeAll
	}
	if len(filter.AppIDs) == 0 {
		filter.AppIDs = []ApplicationID{anyApplicationID}
	}

	sub := &eventSubscription{
		channel: make(chan Event, 100), // TODO: allow custom channel size
		filter:  filter,
		service: s,
	}

	newTopics := make([]string, 0, len(filter.EventTypes))
	for _, evtType := range filter.EventTypes {
		appSubIdx, existingEvtType := s.subscriptions[evtType]
		if !existingEvtType {
			newTopics = append(newTopics, string(evtType))
			appSubIdx = make(applicationToSubscriptionsMap)
			s.subscriptions[evtType] = appSubIdx
		}
		for _, appID := range filter.AppIDs {
			subList, existingAppID := appSubIdx[appID]
			if !existingAppID {
				subList = make(subscriptionList, 0, 10)
			}
			appSubIdx[appID] = append(subList, sub)
		}
	}

	s.Logger.DebugContext(ctx, s.Name+": Added subscription")

	// We add the new subscription to the subscription list optimistically, and
	// it needs to be done this way in case of a restart after an interrupt
	// below has been run, but after a return to this function (say we were to
	// add the new sub at the end of this function, it would not be picked
	// during the restart). But in case of an error subscribing, remove the sub.
	//
	// By the time this function is run (i.e. after an interrupt), a lock on
	// `n.mu` has been reacquired, and modifying subscription state is safe.
	removeSub := func() { s.removeSubscription(ctx, sub) }

	if len(newTopics) > 0 {
		// If already waiting, send an interrupt to the wait function to run a
		// listen operation. If not, connect and listen directly, returning any
		// errors as feedback to the caller.
		if s.isWaiting {
			if err := s.sendInterruptAndReceiveResult(func() error { return s.driverListen(ctx, newTopics) }); err != nil {
				removeSub()
				return nil, err
			}
		} else {
			var justConnected bool

			if !s.isConnected {
				if err := s.driverConnect(ctx, true); err != nil {
					removeSub()
					return nil, err
				}
				justConnected = true
			}

			if err := s.driverListen(ctx, newTopics); err != nil {
				removeSub()

				// If we just connected above and the notifier hasn't started in
				// the interim, also close the connection so we don't leave any
				// resources hanging.
				if justConnected && !s.isStarted {
					s.driverClose(ctx, true)
				}

				return nil, err
			}
		}
	}

	return sub, nil
}

func (s *eventService) unlisten(ctx context.Context, sub *eventSubscription) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	dismissedTopics := make([]string, 0, len(s.subscriptions))
	for _, evtType := range sub.filter.EventTypes {
		appSubIdx := s.subscriptions[evtType]
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
		if s.isWaiting {
			if err := s.sendInterruptAndReceiveResult(func() error { return s.driverUnlisten(ctx, dismissedTopics) }); err != nil {
				return err
			}
		} else {
			if s.isConnected {
				if err := s.driverUnlisten(ctx, dismissedTopics); err != nil {
					return err
				}

				// If this was the last subscription, we weren't in a wait loop,
				// and the service never started, also clean up by closing the
				// driver.
				if !s.isStarted && len(s.subscriptions) <= 1 {
					s.driverClose(ctx, true)
				}
			}
		}
	}

	s.removeSubscription(ctx, sub)

	return nil
}

// This function requires that the caller already has a lock on `n.mu`.
func (s *eventService) removeSubscription(ctx context.Context, sub *eventSubscription) {
	for _, evtType := range sub.filter.EventTypes {
		appSubIdx := s.subscriptions[evtType]
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
			delete(s.subscriptions, evtType)
		}
	}

	s.Logger.DebugContext(
		ctx,
		s.Name+": Removed subscription",
	)
}

func (s *eventService) withLock(lockedFunc func()) {
	s.mu.Lock()
	defer s.mu.Unlock()
	lockedFunc()
}

func (s *eventService) Publish(ctx context.Context, event Event) error {
	payload, err := json.Marshal(event)
	if err != nil {
		return err
	}

	return s.driver.Notify(ctx, &Notification{
		Topic: string(event.Type),
		Payload: string(payload),
	})
}
