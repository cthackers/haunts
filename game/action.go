package game

import (
  "fmt"
  "glop/gui"
)

var action_map map[string]func() Action

func MakeAction(name string) Action {
  f,ok := action_map[name]
  if !ok {
    panic(fmt.Sprintf("Unable to find an Action named '%s'", name))
  }
  return f()
}

var action_makers []func() map[string]func() Action
// Ahahahahahaha
func RegisterActionMakers(f func() map[string]func() Action) {
  action_makers = append(action_makers, f)
}

func RegisterActions() {
  action_map = make(map[string]func() Action)
  for _,maker := range action_makers {
    m := maker()
    for name,f := range m {
      if _,ok := action_map[name]; ok {
        panic(fmt.Sprintf("Tried to register more than one action by the same name: '%s'", name))
      }
      action_map[name] = f
    }
  }
}

// Acceptable values to be returned from Action.Maintain()
type MaintenanceStatus int
const (
  // The Action is in progress and should not be interrupted.
  InProgress         MaintenanceStatus = iota

  // The Action is interrupted and can be interrupted immediately.
  CheckForInterrupts

  // The Action has been completed.
  Complete
)

// Acceptable values to be returned from Action.HandleInput()
type InputStatus int
const (
  // The input was not consumed.
  NotConsumed InputStatus = iota

  // The input was consumed but the action has not begun.
  Consumed

  // The input was consumed and the action has begun.
  ConsumedAndBegin
)

type Action interface {
  // Returns true iff this action can be used as an interrupt
  Readyable() bool

  // Called when the user attempts to select the action.  Returns true if the
  // actions can be performed at least minimally, false if the action cannot
  // be performed at all.
  Prep(*Entity, *Game) bool

  // Got to have some way for the user to interact with the action.  Returns
  // true if the action has been comitted.  If this action is not being
  // readied then it will take effect immediately.  If this function returns
  // ConsumedAndBegin it should charge the required Ap when it does so.
  HandleInput(gui.EventGroup, *Game) InputStatus

  // Got to have some way for the user to see what is going on
  RenderOnFloor()

  // Called if the user cancels the action - done this way so that all actions
  // can be cancelled in the same way instead of each action deciding how to
  // cancel itself.  This method is only called before Maintain() is called
  // the first time, or after Maintain() returns CheckForInterrupts.
  Cancel()

  // Actually executes the action.  Returns a value after every call
  // indicating whether the action is done, still in progress, or can be
  // interrupted.
  Maintain(dt int64) MaintenanceStatus

  // This will be called if the action has been readied at this is a logical
  // point for an interrupt to happen.  Should return true if the action
  // should take place.
  Interrupt() bool
}