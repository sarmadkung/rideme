// Package statemachine enforces status transitions.
//
// Document 15 states the rule this package exists to keep: "Clients cannot
// directly set arbitrary status. Backend commands perform transitions." A
// status column that any handler may assign is not a state machine; it is a
// string that happens to hold state names, and every invalid sequence it
// permits becomes a support case.
//
// Three machines are documented — job (15), driver and vehicle (16) — and more
// arrive with assignments, payments and orders. They differ only in their
// tables, so the table is the input and this is the engine.
package statemachine

import (
	"fmt"
	"sort"
	"strings"
)

// State is a status value.
type State string

// Machine is a set of permitted transitions.
type Machine[S ~string] struct {
	name        string
	transitions map[S]map[S]bool
	terminal    map[S]bool
	initial     S
}

// Definition describes a machine declaratively.
type Definition[S ~string] struct {
	Name    string
	Initial S
	// Transitions maps each state to the states it may move to.
	Transitions map[S][]S
	// Terminal states have no outgoing transitions. Listing them separately
	// makes "is this job finished?" a question about the model rather than a
	// hardcoded list at each call site.
	Terminal []S
}

// New builds a machine, panicking on a definition that contradicts itself.
//
// Panicking is right here: definitions are package-level constants, so a bad
// one is a programming error that should stop the process at startup rather
// than surface as a rejected transition weeks later.
func New[S ~string](def Definition[S]) *Machine[S] {
	m := &Machine[S]{
		name:        def.Name,
		transitions: map[S]map[S]bool{},
		terminal:    map[S]bool{},
		initial:     def.Initial,
	}
	for _, state := range def.Terminal {
		m.terminal[state] = true
	}
	for from, targets := range def.Transitions {
		if m.terminal[from] {
			panic(fmt.Sprintf("statemachine %s: terminal state %v has outgoing transitions", def.Name, from))
		}
		m.transitions[from] = map[S]bool{}
		for _, to := range targets {
			m.transitions[from][to] = true
		}
	}
	return m
}

// Initial is the state a new entity starts in.
func (m *Machine[S]) Initial() S { return m.initial }

// Terminal reports whether a state is final.
func (m *Machine[S]) Terminal(state S) bool { return m.terminal[state] }

// Known reports whether a state exists in this machine.
func (m *Machine[S]) Known(state S) bool {
	if m.terminal[state] || state == m.initial {
		return true
	}
	if _, ok := m.transitions[state]; ok {
		return true
	}
	for _, targets := range m.transitions {
		if targets[state] {
			return true
		}
	}
	return false
}

// Can reports whether a transition is permitted.
func (m *Machine[S]) Can(from, to S) bool {
	return m.transitions[from][to]
}

// Next lists the states reachable from one state, sorted for stable output.
func (m *Machine[S]) Next(from S) []S {
	out := make([]S, 0, len(m.transitions[from]))
	for to := range m.transitions[from] {
		out = append(out, to)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// TransitionError explains a refused transition in terms a caller can act on.
type TransitionError struct {
	Machine string
	From    any
	To      any
	Allowed []string
}

func (e *TransitionError) Error() string {
	if len(e.Allowed) == 0 {
		return fmt.Sprintf("%s: %v is terminal; no transition to %v is possible", e.Machine, e.From, e.To)
	}
	return fmt.Sprintf("%s: cannot move from %v to %v; allowed: %s",
		e.Machine, e.From, e.To, strings.Join(e.Allowed, ", "))
}

// Validate returns nil if the transition is permitted, or a TransitionError
// naming what was allowed instead.
func (m *Machine[S]) Validate(from, to S) error {
	if !m.Known(from) {
		return &TransitionError{Machine: m.name, From: from, To: to}
	}
	if m.Can(from, to) {
		return nil
	}
	allowed := make([]string, 0, len(m.transitions[from]))
	for _, state := range m.Next(from) {
		allowed = append(allowed, string(state))
	}
	return &TransitionError{Machine: m.name, From: from, To: to, Allowed: allowed}
}
