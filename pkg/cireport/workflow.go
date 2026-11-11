package cireport

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// maxWorkflowBytes bounds one workflow file read by the lint.
const maxWorkflowBytes = 1 << 20

// workflow is the part of a workflow file the lint reads.
type workflow struct {
	on          *yaml.Node
	concurrency concurrency
	jobs        []wfJob
}

type concurrency struct {
	set    bool
	group  string
	cancel string // "true", "false", "expr", or "" when absent
}

type wfJob struct {
	id, name, ifCond string
	labels           []string
	runsOnExpr       bool
	hasSteps         bool
	uses             string
	timeout          bool
	checkoutRefs     []string
}

// LoadWorkflows reads the *.yml and *.yaml files directly in dir.
func LoadWorkflows(dir string) (map[string][]byte, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	files := map[string][]byte{}
	for _, e := range entries {
		ext := filepath.Ext(e.Name())
		if !e.Type().IsRegular() || (ext != ".yml" && ext != ".yaml") {
			continue
		}
		info, err := e.Info()
		if err != nil || info.Size() > maxWorkflowBytes {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err == nil {
			files[e.Name()] = data
		}
	}
	return files, nil
}

func parseWorkflow(data []byte) (*workflow, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, err
	}
	if len(doc.Content) == 0 || doc.Content[0].Kind != yaml.MappingNode {
		return nil, errors.New("not a mapping")
	}
	wf := &workflow{}
	for key, val := range pairs(doc.Content[0]) {
		switch key {
		// A YAML 1.1 reader turns the key "on" into true; accept both.
		case "on", "true":
			wf.on = val
		case "concurrency":
			wf.concurrency = parseConcurrency(val)
		case "jobs":
			for id, job := range pairs(val) {
				wf.jobs = append(wf.jobs, parseJob(id, job))
			}
		}
	}
	sort.Slice(wf.jobs, func(i, j int) bool { return wf.jobs[i].id < wf.jobs[j].id })
	return wf, nil
}

// pairs iterates the key/value pairs of a mapping node.
func pairs(n *yaml.Node) func(func(string, *yaml.Node) bool) {
	return func(yield func(string, *yaml.Node) bool) {
		if n == nil || n.Kind != yaml.MappingNode {
			return
		}
		for i := 0; i+1 < len(n.Content); i += 2 {
			if !yield(n.Content[i].Value, n.Content[i+1]) {
				return
			}
		}
	}
}

func scalars(n *yaml.Node) []string {
	switch {
	case n == nil:
		return nil
	case n.Kind == yaml.ScalarNode:
		return []string{n.Value}
	case n.Kind == yaml.SequenceNode:
		var out []string
		for _, c := range n.Content {
			if c.Kind == yaml.ScalarNode {
				out = append(out, c.Value)
			}
		}
		return out
	}
	return nil
}

// triggers are the event names of the on: key, sorted.
func (w *workflow) triggers() []string {
	var out []string
	if w.on != nil && w.on.Kind == yaml.MappingNode {
		for key := range pairs(w.on) {
			out = append(out, key)
		}
	} else {
		out = scalars(w.on)
	}
	sort.Strings(out)
	return out
}

func (w *workflow) has(events ...string) bool {
	for _, t := range w.triggers() {
		for _, e := range events {
			if t == e {
				return true
			}
		}
	}
	return false
}

func parseConcurrency(n *yaml.Node) concurrency {
	c := concurrency{set: n != nil}
	if n != nil && n.Kind == yaml.ScalarNode {
		c.group = n.Value
		return c
	}
	for key, val := range pairs(n) {
		switch key {
		case "group":
			c.group = val.Value
		case "cancel-in-progress":
			c.cancel = strings.ToLower(strings.TrimSpace(val.Value))
			if strings.Contains(c.cancel, "${{") {
				c.cancel = "expr"
			}
		}
	}
	return c
}

func parseJob(id string, n *yaml.Node) wfJob {
	j := wfJob{id: id}
	for key, val := range pairs(n) {
		switch key {
		case "name":
			j.name = val.Value
		case "if":
			j.ifCond = val.Value
		case "uses":
			j.uses = val.Value
		case "timeout-minutes":
			j.timeout = true
		case "runs-on":
			j.labels = scalars(val)
			for k, v := range pairs(val) {
				if k == "group" {
					j.labels = append(j.labels, "group:"+v.Value)
				} else if k == "labels" {
					j.labels = append(j.labels, scalars(v)...)
				}
			}
		case "steps":
			j.hasSteps = val.Kind == yaml.SequenceNode && len(val.Content) > 0
			for _, step := range val.Content {
				j.checkoutRefs = append(j.checkoutRefs, checkoutRef(step)...)
			}
		}
	}
	for _, l := range j.labels {
		j.runsOnExpr = j.runsOnExpr || strings.Contains(l, "${{")
	}
	return j
}

// checkoutRef is the with.ref of an actions/checkout step, if any.
func checkoutRef(step *yaml.Node) []string {
	var uses string
	var with *yaml.Node
	for key, val := range pairs(step) {
		switch key {
		case "uses":
			uses = val.Value
		case "with":
			with = val
		}
	}
	if !strings.HasPrefix(uses, "actions/checkout") {
		return nil
	}
	for key, val := range pairs(with) {
		if key == "ref" {
			return []string{val.Value}
		}
	}
	return nil
}

func (j wfJob) label() string {
	if j.name != "" && j.name != j.id {
		return fmt.Sprintf("%s (%s)", j.id, j.name)
	}
	return j.id
}
