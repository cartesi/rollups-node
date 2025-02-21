// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

// Finite State Machine algorithm where the possible actions are:
// - (update)  database
// - (submit)  to blockchain
// - (invalid) or constraint violation of the state/event sequence
// - (none)    nothing to do
//
// In table format:
//
// | n |      prev     |      curr     | action |
// |   | state | event | state | event |        |
// |---+-------+-------+-------+-------+--------+
// | 1 |   .   |   .   |  cs   |   .   | submit |
// | 2 |   .   |   .   |  cs   |  ce   | update |
// | 3 |  ps   |  pe   |  cs   |   .   | submit |
// | 4 |  ps   |  pe   |  cs   |  ce   | update |
//
// Note: Submit operations may take a long time to complete.
//       Until an event shows up, the action will still be `submit`.
//       To avoid submitting more than once, we suggest tracking events in a
//       "in flight" data structure and skipping tryTransition while they do.
//
// 1. On startup there are no previous states nor events. It should:
//
//   - Emit a `submit` action on transition from current state.
//
// 2. Some time after the submission, there will be a confirmation event.
//
//   - Event must match the state, then emit a `update` action.
//
// 3. Folloing states transitions require additional checks. Same as (1) otherwise.
// 3.1. No event was skipped:
//   - checkStateConstraint(prevState, currState)
//
// 4. After the first epoch, additional checks must be done. Same as (2) otherwise.
// 4.1. epochs are in order:
//   - previous_claim.last_block < current_claim.first_block
//
// 4.2. There are no events between the epochs
//   - next(previous_event) == current_event
//
// Other cases are errors.
package fsm

import "fmt"

type Action int
const (
	None Action = iota
	Submit
	Update
	Invalid
)

func (me Action) String() string {
	switch (me) {
	case None:   return "none"
	case Submit: return "submit"
	case Update: return "update"
	default:     return "invalid"
	}
}

var (
	ErrStateConstraintViolation  = fmt.Errorf("invalid current state")
	ErrStateTransitionConstraintViolation = fmt.Errorf("state transition constraint violation")
	ErrEventConstraintViolation  = fmt.Errorf("event constraint violation")
)

type FSM[S any, E any] interface{
	FetchEventAndSucc(*S) (*E, *E, error)

	CheckStateConstraint(state *S) error
	CheckEventConstraint(event *E) error

	CheckStateTransitionConstraint(prev *S, curr *S) error
	CheckEventTransitionConstraint(state *S, event *E) error
}

func TryTransition[S any, E any, T FSM[S, E]](
	fsm T,
	prevState *S,
	currState *S,
) (
	Action,
	*E,
	*E,
	error,
) {
	var prevEvent, currEvent *E
	var err error

	if err = fsm.CheckStateConstraint(currState); err != nil {
		return Invalid, nil, nil, err
	}

	if prevState == nil {
		// there is no previous state, fetch events of current only
		currEvent, _, err = fsm.FetchEventAndSucc(currState)
		if err != nil {
			return Invalid, nil, nil, err
		}
	} else {
		// there is a previous state, ensure current state comes after it
		if err = fsm.CheckStateConstraint(prevState); err != nil {
			return Invalid, nil, nil, err
		}
		if err = fsm.CheckStateTransitionConstraint(prevState, currState); err != nil {
			return Invalid, nil, nil, err
		}

		// if there is a previous state: previous event must match it
		prevEvent, currEvent, err = fsm.FetchEventAndSucc(prevState)
		if err != nil {
			return Invalid, nil, nil, err
		}
		if err = fsm.CheckEventConstraint(prevEvent); err != nil {
			return Invalid, prevEvent, currEvent, err
		}
		if err = fsm.CheckEventTransitionConstraint(prevState, prevEvent); err != nil {
			return Invalid, prevEvent, currEvent, err
		}
	}

	if currEvent != nil {
		if err = fsm.CheckEventConstraint(currEvent); err != nil {
			return Invalid, prevEvent, currEvent, err
		}
		return Update, prevEvent, currEvent, fsm.CheckEventTransitionConstraint(currState, currEvent)
	} else {
		return Submit, prevEvent, currEvent, nil
	}
}
