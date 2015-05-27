package main

import (
	"bytes"
	"fmt"
	"os"
	"strings"
)

type NinjaGenerator struct {
	f      *os.File
	nodes  []*DepNode
	vars   Vars
	ex     *Executor
	ruleId int
	done   map[string]bool
}

func NewNinjaGenerator(g *DepGraph) *NinjaGenerator {
	f, err := os.Create("build.ninja")
	if err != nil {
		panic(err)
	}
	return &NinjaGenerator{
		f:     f,
		nodes: g.nodes,
		vars:  g.vars,
		done:  make(map[string]bool),
	}
}

func genShellScript(runners []runner) string {
	var buf bytes.Buffer
	for i, r := range runners {
		if i > 0 {
			if runners[i-1].ignoreError {
				buf.WriteString(" ; ")
			} else {
				buf.WriteString(" && ")
			}
		}
		cmd := trimLeftSpace(r.cmd)
		cmd = strings.Replace(cmd, "\\\n", " ", -1)
		cmd = strings.TrimRight(cmd, " \t\n;")
		cmd = strings.Replace(cmd, "$", "$$", -1)
		cmd = strings.Replace(cmd, "\t", " ", -1)
		buf.WriteString(cmd)
		if i == len(runners)-1 && r.ignoreError {
			buf.WriteString(" ; true")
		}
	}
	return buf.String()
}

func (n *NinjaGenerator) genRuleName() string {
	ruleName := fmt.Sprintf("rule%d", n.ruleId)
	n.ruleId++
	return ruleName
}

func (n *NinjaGenerator) emitBuild(output, rule, dep string) {
	fmt.Fprintf(n.f, "build %s: %s", output, rule)
	if dep != "" {
		fmt.Fprintf(n.f, " %s", dep)
	}
	fmt.Fprintf(n.f, "\n")
}

func getDepString(node *DepNode) string {
	var deps []string
	var orderOnlys []string
	for _, d := range node.Deps {
		if d.IsOrderOnly {
			orderOnlys = append(orderOnlys, d.Output)
		} else {
			deps = append(deps, d.Output)
		}
	}
	dep := ""
	if len(deps) > 0 {
		dep += fmt.Sprintf(" %s", strings.Join(deps, " "))
	}
	if len(orderOnlys) > 0 {
		dep += fmt.Sprintf(" || %s", strings.Join(orderOnlys, " "))
	}
	return dep
}

func genIntermediateTargetName(o string, i int) string {
	return fmt.Sprintf(".make_targets/%s@%d", o, i)
}

func (n *NinjaGenerator) emitNode(node *DepNode) {
	if n.done[node.Output] {
		return
	}
	n.done[node.Output] = true

	if len(node.Cmds) == 0 && len(node.Deps) == 0 && !node.IsPhony {
		return
	}

	runners, _ := n.ex.createRunners(node, true)
	ruleName := "phony"
	if len(runners) > 0 {
		ruleName = n.genRuleName()
		fmt.Fprintf(n.f, "rule %s\n", ruleName)
		fmt.Fprintf(n.f, " description = build $out\n")

		ss := genShellScript(runners)
		// It seems Linux is OK with ~130kB.
		// TODO: Find this number automatically.
		ArgLenLimit := 100 * 1000
		if len(ss) > ArgLenLimit {
			fmt.Fprintf(n.f, " rspfile = $out.rsp\n")
			fmt.Fprintf(n.f, " rspfile_content = %s\n", ss)
			ss = "sh $out.rsp"
		}
		fmt.Fprintf(n.f, " command = %s\n", ss)

	}
	n.emitBuild(node.Output, ruleName, getDepString(node))

	for _, d := range node.Deps {
		n.emitNode(d)
	}
}

func (n *NinjaGenerator) run() {
	n.ex = NewExecutor(n.vars)
	for _, node := range n.nodes {
		n.emitNode(node)
	}
	n.f.Close()
}

func GenerateNinja(g *DepGraph) {
	NewNinjaGenerator(g).run()
}