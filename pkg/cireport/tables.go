package cireport

import "sort"

type jobAcc struct {
	run, queue []float64
	runSum     float64
	queueSum   float64
}

func (a *jobAcc) add(j Job) {
	if j.Conclusion == "skipped" {
		return
	}
	a.run = append(a.run, j.RunSeconds())
	a.runSum += j.RunSeconds()
	if q, ok := j.QueueSeconds(); ok {
		a.queue = append(a.queue, q)
		a.queueSum += q
	}
}

func jobStats(byJob map[[2]string]*jobAcc, scale, days float64) []JobStat {
	out := []JobStat{}
	for key, a := range byJob {
		if len(a.run) == 0 {
			continue
		}
		out = append(out, JobStat{Workflow: key[0], Job: key[1], Count: len(a.run),
			RunMedianSec: percentile(a.run, 0.5), RunP90Sec: percentile(a.run, 0.9),
			QueueMedianSec: percentile(a.queue, 0.5), QueueP90Sec: percentile(a.queue, 0.9),
			MinutesPerDay: a.runSum / 60 * scale / days, QueueMinutesPerDay: a.queueSum / 60 * scale / days})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].MinutesPerDay != out[j].MinutesPerDay {
			return out[i].MinutesPerDay > out[j].MinutesPerDay
		}
		return out[i].Workflow+"/"+out[i].Job < out[j].Workflow+"/"+out[j].Job
	})
	return out
}

func eventStats(byEvent map[string][]float64) []EventStat {
	out := []EventStat{}
	for event, values := range byEvent {
		out = append(out, EventStat{Event: event, Runs: len(values), MedianSec: percentile(values, 0.5), P90Sec: percentile(values, 0.9)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Event < out[j].Event })
	return out
}

func runnerStats(byRunner map[string]*RunnerStat, scale, days float64) []RunnerStat {
	out := []RunnerStat{}
	for _, r := range byRunner {
		stat := *r
		stat.BusyMinutesPerDay = r.BusyMinutesPerDay * scale / days
		// A hosted label is a pool, not one machine: no share of a day.
		if stat.SelfHosted {
			stat.Utilisation = stat.BusyMinutesPerDay / 1440
		}
		out = append(out, stat)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].BusyMinutesPerDay != out[j].BusyMinutesPerDay {
			return out[i].BusyMinutesPerDay > out[j].BusyMinutesPerDay
		}
		return out[i].Runner < out[j].Runner
	})
	return out
}

func conclusionStats(byConclusion map[string]*ConclusionStat, scale float64) []ConclusionStat {
	out := []ConclusionStat{}
	for _, c := range byConclusion {
		out = append(out, ConclusionStat{Conclusion: c.Conclusion, Jobs: c.Jobs, Minutes: round3(c.Minutes * scale)})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Minutes != out[j].Minutes {
			return out[i].Minutes > out[j].Minutes
		}
		return out[i].Conclusion < out[j].Conclusion
	})
	return out
}
