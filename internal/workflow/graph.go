package workflow

import (
	"fmt"
	"my-go-agent/internal/agent"
)

type Node struct {
	Name        string
	Description string

	agent    agent.Agent
	next     []*Node
	inDegree int
}

func NewNode(agent agent.Agent, name string, description string) *Node {
	return &Node{agent: agent, Name: name, Description: description}
}

type Graph struct {
	root  *Node
	nodes map[string]*Node
}

func NewGraph() *Graph {
	return &Graph{}
}

func (g *Graph) SetRoot(name string) error {
	node, ok := g.nodes[name]
	if !ok {
		return fmt.Errorf("node %s not found", name)
	}
	g.root = node
	return nil
}

func (g *Graph) RenameNode(old string, now string) error {
	node, ok := g.nodes[old]
	if !ok {
		return fmt.Errorf("node %s not found", old)
	}
	node.Name = now
	delete(g.nodes, old)
	g.nodes[now] = node
	return nil
}
func (g *Graph) AddNodes(nodes ...*Node) {
	for _, node := range nodes {
		g.nodes[node.Name] = node
	}
}

func (g *Graph) AddEdge(src string, dst string) error {
	from, ok := g.nodes[src]
	if !ok {
		return fmt.Errorf("node %s not found", src)
	}

	to, ok := g.nodes[dst]
	if !ok {
		return fmt.Errorf("node %s not found", dst)
	}

	from.next = append(from.next, to)
	to.inDegree++

	return nil
}

func (g *Graph) Run() error {
	
}
