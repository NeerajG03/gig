// Package gig is a lightweight task management SDK backed by SQLite.
//
// # File Layout
//
//	gig.go         – Domain types: Task, Status, Priority, Event, Attribute, etc.
//	store.go       – Store constructor (Open/Close), ID generation, event system
//	task.go        – Task mutations: Create, Get, Update, Close, Reopen, Claim
//	query.go       – Task queries: List, Search, Ready, Blocked, Children, GetTree
//	dependency.go  – Dependency DAG: Add, Remove, cycle detection, tree visualization
//	attribute.go   – Custom attributes: Define, Set, Get, type validation
//	comment.go     – Comments: Add, List
//	event.go       – Event queries: Events, EventsSince
//	hook.go        – Shell hook execution from config
//	config.go      – Config loading (gig.yaml), defaults, paths
//	export.go      – JSONL export/import for sync
//	util.go        – Time parsing, label serialization helpers
package gig
