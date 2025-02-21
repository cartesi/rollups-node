// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package fsm

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// implementation of the most basic State + Event pair
type FSMMock struct {
	mock.Mock
}

type ID uint64
type State struct{}
type Event struct{}

var (
	nilEvent = (*Event)(nil)
	nilState = (*State)(nil)
	ErrFetch = fmt.Errorf("Failed to fetch event")
)

////////////////////////////////////////////////////////////////////////////////
func (m *FSMMock) CheckStateConstraint(
	state *State,
) error {
	args := m.Called(state)
	return args.Error(0)
}

func (m *FSMMock) CheckEventConstraint(
	event *Event,
) error {
	args := m.Called(event)
	return args.Error(0)
}

func (m *FSMMock) CheckStateTransitionConstraint(
	prev *State,
	curr *State,
) error {
	args := m.Called(prev, curr)
	return args.Error(0)
}

func (m *FSMMock) CheckEventTransitionConstraint(
	state *State,
	event *Event,
) error {
	args := m.Called(state, event)
	return args.Error(0)
}
func (m *FSMMock) FetchEventAndSucc(
	state *State,
) (
	*Event,
	*Event,
	error,
) {
	args := m.Called(state)
	return args.Get(0).(*Event),
		args.Get(1).(*Event),
		args.Error(2)
}

////////////////////////////////////////////////////////////////////////////////
// success path
////////////////////////////////////////////////////////////////////////////////
func TestFirstStateNoEvent(t *testing.T) {
	currState := State{}
	fsm := FSMMock{}
	fsm.On("CheckStateConstraint", &currState).Return(nil).Once()
	fsm.On("FetchEventAndSucc", &currState).Return(nilEvent, nilEvent, nil).Once()

	action, _, _, err := TryTransition(&fsm, nil, &currState)
	assert.Equal(t, nil, err)
	assert.Equal(t, Submit, action)
}

func TestFollowStateNoEvent(t *testing.T) {
	prevState := State{}
	prevEvent := Event{}
	currState := State{}
	fsm := FSMMock{}
	fsm.On("CheckStateConstraint", &currState).Return(nil).Once()
	fsm.On("CheckStateConstraint", &prevState).Return(nil).Once()
	fsm.On("FetchEventAndSucc", &currState).Return(&prevEvent, nilEvent, nil).Once()
	fsm.On("CheckStateTransitionConstraint", &prevState, &currState).Return(nil).Once()
	fsm.On("CheckEventConstraint", &prevEvent).Return(nil).Once()
	fsm.On("CheckEventTransitionConstraint", &prevState, &prevEvent).Return(nil).Once()

	action, _, _, err := TryTransition(&fsm, &prevState, &currState)
	assert.Equal(t, nil, err)
	assert.Equal(t, Submit, action)
}

func TestFirstStateWithEvent(t *testing.T) {
	currState := State{}
	currEvent := Event{}
	fsm := FSMMock{}
	fsm.On("CheckStateConstraint", &currState).Return(nil)
	fsm.On("FetchEventAndSucc", &currState).Return(&currEvent, nilEvent, nil)
	fsm.On("CheckEventConstraint", &currEvent).Return(nil)
	fsm.On("CheckEventTransitionConstraint", &currState, &currEvent).Return(nil)

	action, _, _, err := TryTransition(&fsm, nil, &currState)
	assert.Equal(t, nil, err)
	assert.Equal(t, Update, action)
}

func TestFollowStateWithEvent(t *testing.T) {
	prevState := State{}
	prevEvent := Event{}
	currState := State{}
	currEvent := Event{}
	fsm := FSMMock{}
	fsm.On("CheckStateConstraint", &currState).Return(nil).Once()
	fsm.On("CheckStateConstraint", &prevState).Return(nil).Once()
	fsm.On("FetchEventAndSucc", &currState).Return(&prevEvent, &currEvent, nil).Once()
	fsm.On("CheckEventConstraint", &prevEvent).Return(nil).Once()
	fsm.On("CheckEventConstraint", &currEvent).Return(nil).Once()
	fsm.On("CheckEventTransitionConstraint", &prevState, &prevEvent).Return(nil).Once()
	fsm.On("CheckStateTransitionConstraint", &prevState, &currState).Return(nil).Once()
	fsm.On("CheckEventTransitionConstraint", &currState, &currEvent).Return(nil).Once()

	action, _, _, err := TryTransition(&fsm, &prevState, &currState)
	assert.Equal(t, nil, err)
	assert.Equal(t, Update, action)
}

func TestFormat(t *testing.T) {
	assert.Equal(t, "none",   None.String())
	assert.Equal(t, "submit", Submit.String())
	assert.Equal(t, "update", Update.String())
	assert.Equal(t, "invalid", Action(-1).String())
}
////////////////////////////////////////////////////////////////////////////////
// error path
////////////////////////////////////////////////////////////////////////////////

func TestFirstStateWithEventError0(t *testing.T) {
	currState := State{}
	currEvent := Event{}
	fsm := FSMMock{}
	fsm.On("CheckStateConstraint", &currState).Return(ErrStateConstraintViolation).Once()
	fsm.On("CheckEventConstraint", &currEvent).Return(nil).Once()
	fsm.On("CheckEventTransitionConstraint", &currState, &currEvent).Return(nil).Once()

	_, _, _, err := TryTransition(&fsm, nilState, &currState)
	assert.Equal(t, ErrStateConstraintViolation, err)
}

func TestFirstStateWithEventError1(t *testing.T) {
	currState := State{}
	currEvent := Event{}
	fsm := FSMMock{}
	fsm.On("CheckStateConstraint", &currState).Return(nil).Once()
	fsm.On("CheckEventConstraint", &currEvent).Return(nil).Once()
	fsm.On("FetchEventAndSucc", &currState).Return(nilEvent, &currEvent, ErrFetch).Once()
	fsm.On("CheckEventTransitionConstraint", &currState, &currEvent).Return(nil).Once()

	_, _, _, err := TryTransition(&fsm, nilState, &currState)
	assert.Equal(t, ErrFetch, err)
}

func TestFollowStateWithEventError0(t *testing.T) {
	prevState := State{}
	prevEvent := Event{}
	currState := State{}
	currEvent := Event{}
	fsm := FSMMock{}
	fsm.On("CheckStateConstraint", &currState).Return(ErrStateConstraintViolation).Once()
	fsm.On("CheckStateConstraint", &prevState).Return(nil).Once()
	fsm.On("FetchEventAndSucc", &currState).Return(&prevEvent, &currEvent, nil).Once()
	fsm.On("CheckEventConstraint", &prevEvent).Return(nil).Once()
	fsm.On("CheckEventConstraint", &currEvent).Return(nil).Once()
	fsm.On("CheckStateTransitionConstraint", &prevState, &currState).Return(nil).Once()
	fsm.On("CheckEventTransitionConstraint", &currState, &currEvent).Return(nil).Once()

	_, _, _, err := TryTransition(&fsm, &prevState, &currState)
	assert.Equal(t, ErrStateConstraintViolation, err)
}

func TestFollowStateWithEventError1(t *testing.T) {
	prevState := State{}
	prevEvent := Event{}
	currState := State{}
	currEvent := Event{}
	fsm := FSMMock{}
	fsm.On("CheckStateConstraint", &currState).Return(nil).Once()
	fsm.On("CheckStateConstraint", &prevState).Return(ErrStateConstraintViolation).Once()
	fsm.On("FetchEventAndSucc", &currState).Return(&prevEvent, &currEvent, nil).Once()
	fsm.On("CheckEventConstraint", &prevEvent).Return(nil).Once()
	fsm.On("CheckEventConstraint", &currEvent).Return(nil).Once()
	fsm.On("CheckStateTransitionConstraint", &prevState, &currState).Return(nil).Once()
	fsm.On("CheckEventTransitionConstraint", &currState, &currEvent).Return(nil).Once()

	_, _, _, err := TryTransition(&fsm, &prevState, &currState)
	assert.Equal(t, ErrStateConstraintViolation, err)
}

func TestFollowStateWithEventError2(t *testing.T) {
	prevState := State{}
	prevEvent := Event{}
	currState := State{}
	currEvent := Event{}
	fsm := FSMMock{}
	fsm.On("CheckStateConstraint", &currState).Return(nil).Once()
	fsm.On("CheckStateConstraint", &prevState).Return(nil).Once()
	fsm.On("FetchEventAndSucc", &currState).Return(&prevEvent, &currEvent, ErrFetch).Once()
	fsm.On("CheckEventConstraint", &prevEvent).Return(nil).Once()
	fsm.On("CheckEventConstraint", &currEvent).Return(nil).Once()
	fsm.On("CheckStateTransitionConstraint", &prevState, &currState).Return(nil).Once()
	fsm.On("CheckEventTransitionConstraint", &currState, &currEvent).Return(nil).Once()

	_, _, _, err := TryTransition(&fsm, &prevState, &currState)
	assert.Equal(t, ErrFetch, err)
}

func TestFollowStateWithEventError3(t *testing.T) {
	prevState := State{}
	prevEvent := Event{}
	currState := State{}
	currEvent := Event{}
	fsm := FSMMock{}
	fsm.On("CheckStateConstraint", &currState).Return(nil)
	fsm.On("CheckStateConstraint", &prevState).Return(nil)
	fsm.On("FetchEventAndSucc", &currState).Return(&prevEvent, &currEvent, nil)
	fsm.On("CheckEventConstraint", &prevEvent).Return(ErrEventConstraintViolation)
	fsm.On("CheckEventConstraint", &currEvent).Return(nil)
	fsm.On("CheckStateTransitionConstraint", &prevState, &currState).Return(nil)
	fsm.On("CheckEventTransitionConstraint", &currState, &currEvent).Return(nil)

	_, _, _, err := TryTransition(&fsm, &prevState, &currState)
	assert.Equal(t, ErrEventConstraintViolation, err)
}

func TestFollowStateWithEventError4(t *testing.T) {
	prevState := State{}
	prevEvent := Event{}
	currState := State{}
	currEvent := Event{}
	fsm := FSMMock{}
	fsm.On("CheckStateConstraint", &currState).Return(nil).Once()
	fsm.On("CheckStateConstraint", &prevState).Return(nil).Once()
	fsm.On("FetchEventAndSucc", &currState).Return(&prevEvent, &currEvent, nil).Once()
	fsm.On("CheckEventConstraint", &prevEvent).Return(nil).Once()
	fsm.On("CheckEventConstraint", &currEvent).Return(ErrEventConstraintViolation).Once()
	fsm.On("CheckStateTransitionConstraint", &prevState, &currState).Return(nil).Once()
	fsm.On("CheckEventTransitionConstraint", &currState, &currEvent).Return(nil).Once()

	_, _, _, err := TryTransition(&fsm, &prevState, &currState)
	assert.Equal(t, ErrEventConstraintViolation, err)
}

func TestFollowStateWithEventError5(t *testing.T) {
	prevState := State{}
	prevEvent := Event{}
	currState := State{}
	currEvent := Event{}
	fsm := FSMMock{}
	fsm.On("CheckStateConstraint", &currState).Return(nil).Once()
	fsm.On("CheckStateConstraint", &prevState).Return(nil).Once()
	fsm.On("FetchEventAndSucc", &currState).Return(&prevEvent, &currEvent, nil).Once()
	fsm.On("CheckEventConstraint", &prevEvent).Return(nil).Once()
	fsm.On("CheckEventConstraint", &currEvent).Return(nil).Once()
	fsm.On("CheckStateTransitionConstraint", &prevState, &currState).
		Return(ErrStateTransitionConstraintViolation).Once()
	fsm.On("CheckEventTransitionConstraint", &currState, &currEvent).Return(nil).Once()

	_, _, _, err := TryTransition(&fsm, &prevState, &currState)
	assert.Equal(t, ErrStateTransitionConstraintViolation, err)
}

func TestFollowStateWithEventError6(t *testing.T) {
	prevState := State{}
	prevEvent := Event{}
	currState := State{}
	currEvent := Event{}
	fsm := FSMMock{}
	fsm.On("CheckStateConstraint", &currState).Return(nil).Once()
	fsm.On("CheckStateConstraint", &prevState).Return(nil).Once()
	fsm.On("FetchEventAndSucc", &currState).Return(&prevEvent, &currEvent, nil).Once()
	fsm.On("CheckEventConstraint", &prevEvent).Return(nil).Once()
	fsm.On("CheckEventConstraint", &currEvent).Return(nil).Once()
	fsm.On("CheckStateTransitionConstraint", &prevState, &currState).Return(nil).Once()
	fsm.On("CheckEventTransitionConstraint", &currState, &currEvent).
		Return(ErrStateTransitionConstraintViolation).Once()

	_, _, _, err := TryTransition(&fsm, &prevState, &currState)
	assert.Equal(t, ErrStateTransitionConstraintViolation, err)
}
