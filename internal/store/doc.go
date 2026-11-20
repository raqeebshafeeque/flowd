// Package store keeps days, items and their events in one SQLite file.
//
// Every change to an item is saved together with its event in a single
// transaction, and the whole day is checked against the domain rules before
// the transaction commits. Events can never be updated or deleted.
//
// Only one operation owns the file at a time. A damaged file, an unknown
// schema or an unsafe path is refused and left exactly as it was. The store
// never prints anything. It returns errors with their original cause.
package store
