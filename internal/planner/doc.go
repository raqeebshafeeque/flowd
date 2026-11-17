// Package planner holds the operations every frontend shares: reading a day
// and planning, tracking or deleting its items.
//
// Each operation opens the store, captures the clock once, closes any day
// whose midnight has passed, and only then reads or changes anything. The
// store is closed again before the operation returns, so nothing holds the
// database while a person is thinking.
//
// A change names the item revision it was based on. If the item changed in
// the meantime the change is refused instead of overwriting newer work. When
// a save cannot be confirmed, the result says so, and Reconcile can later
// prove from the saved history whether it happened.
package planner
