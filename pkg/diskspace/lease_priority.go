package diskspace

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// Priority classes of the CPU queue, lowest first. They order admission
// only: a running holder is never preempted.
const (
	PriorityBackground  = "background"
	PriorityCI          = "ci"
	PriorityInteractive = "interactive"
)

var priorityNames = []string{PriorityBackground, PriorityCI, PriorityInteractive}

// ParsePriority normalises a class name; empty means ci.
func ParsePriority(s string) (string, error) {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" {
		return PriorityCI, nil
	}
	for _, name := range priorityNames {
		if s == name {
			return name, nil
		}
	}
	return "", fmt.Errorf("priority %q must be interactive, ci, or background", s)
}

// priorityRank is 0 for background, 1 for ci (and anything unknown, such as
// a ticket written before classes existed), 2 for interactive.
func priorityRank(name string) int {
	switch name {
	case PriorityBackground:
		return 0
	case PriorityInteractive:
		return 2
	}
	return 1
}

// effectiveRank raises a waiter one class per aging interval it has waited
// since it was queued, up to interactive, so no class can starve another.
// An interval of 0 turns aging off.
func effectiveRank(w LeaseInfo, now time.Time, aging time.Duration) int {
	rank := priorityRank(w.Priority)
	if aging > 0 && now.After(w.Started) {
		rank += int(now.Sub(w.Started) / aging)
	}
	return min(rank, len(priorityNames)-1)
}

// grantedBefore reports whether waiter a is admitted before b: the higher
// effective class first, then first in, first out by queue sequence.
func grantedBefore(a, b LeaseInfo, now time.Time, aging time.Duration) bool {
	ra, rb := effectiveRank(a, now, aging), effectiveRank(b, now, aging)
	if ra != rb {
		return ra > rb
	}
	return a.Seq < b.Seq
}

// RankWaiters returns the waiters in grant order at now, each with its
// EffectivePriority set.
func RankWaiters(waiters []LeaseInfo, now time.Time, aging time.Duration) []LeaseInfo {
	out := append([]LeaseInfo(nil), waiters...)
	for i := range out {
		if out[i].Priority == "" {
			out[i].Priority = PriorityCI
		}
		out[i].EffectivePriority = priorityNames[effectiveRank(out[i], now, aging)]
	}
	sort.SliceStable(out, func(i, j int) bool { return grantedBefore(out[i], out[j], now, aging) })
	return out
}
