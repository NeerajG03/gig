package gig

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// RunHooks executes shell hooks matching the given event.
// Hooks are non-blocking (fire-and-forget goroutines).
func (s *Store) RunHooks(event Event) {
	if s.config == nil {
		return
	}

	var hooks []HookDef

	switch event.Type {
	case EventStatusChanged:
		hooks = s.config.Hooks.OnStatusChange
	case EventCreated:
		hooks = s.config.Hooks.OnCreate
	case EventCommented:
		hooks = s.config.Hooks.OnComment
	case EventClosed:
		hooks = s.config.Hooks.OnClose
	case EventAssigned:
		hooks = s.config.Hooks.OnAssign
	}

	for _, h := range hooks {
		if !hookMatchesFilter(h, event) {
			continue
		}
		cmd := expandHookVars(h.Command, event)
		go executeHook(cmd)
	}
}

// hookMatchesFilter checks if a hook's filter conditions match the event.
func hookMatchesFilter(h HookDef, e Event) bool {
	for key, val := range h.Filter {
		switch key {
		case "new_status":
			if e.NewValue != val {
				return false
			}
		case "old_status":
			if e.OldValue != val {
				return false
			}
		case "assignee":
			if e.Actor != val {
				return false
			}
		}
	}
	return true
}

// expandHookVars replaces template variables in a hook command.
func expandHookVars(cmd string, e Event) string {
	r := strings.NewReplacer(
		"{id}", e.TaskID,
		"{old}", e.OldValue,
		"{new}", e.NewValue,
		"{actor}", e.Actor,
		"{field}", e.Field,
	)
	return r.Replace(cmd)
}

// executeHook runs a shell command and logs any errors.
func executeHook(cmd string) {
	c := exec.Command("sh", "-c", cmd)
	c.Stdout = os.Stdout

	logFile := filepath.Join(DefaultGigHome(), "hooks.log")
	f, err := os.OpenFile(logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err == nil {
		c.Stderr = f
		defer f.Close()
	} else {
		c.Stderr = os.Stderr
	}

	if err := c.Run(); err != nil {
		log.Printf("hook error: %s: %v", cmd, err)
		if f != nil {
			fmt.Fprintf(f, "hook error: %s: %v\n", cmd, err)
		}
	}
}

// EnableHooks wires up the store's event system to fire shell hooks.
// Call this after opening a store with a config that has hooks defined.
func (s *Store) EnableHooks() {
	if s.config == nil {
		return
	}

	// Register a catch-all listener that dispatches to shell hooks.
	for _, et := range []EventType{
		EventCreated, EventStatusChanged, EventCommented, EventClosed, EventAssigned,
	} {
		eventType := et
		s.On(eventType, func(e Event) {
			s.RunHooks(e)
		})
	}
}
