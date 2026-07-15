package workflow

import (
	"context"
	"fmt"
	"my-go-agent/internal/agent"
	"my-go-agent/internal/memory"
	"sync"
	"time"
)

// GraphEventType 表示图执行中的事件类型
type GraphEventType int

const (
	GraphNodeStarted   GraphEventType = iota // 节点开始执行
	GraphNodeCompleted                       // 节点执行成功
	GraphNodeFailed                          // 节点执行失败
)

// String 返回事件类型的可读字符串
func (e GraphEventType) String() string {
	switch e {
	case GraphNodeStarted:
		return "node_started"
	case GraphNodeCompleted:
		return "node_completed"
	case GraphNodeFailed:
		return "node_failed"
	default:
		return "unknown"
	}
}

// GraphEvent 携带图执行过程中的事件信息
type GraphEvent struct {
	Type      GraphEventType
	NodeName  string
	Output    string
	Err       error
	Timestamp time.Time
}

// GraphEventListener 图执行事件监听器函数类型
type GraphEventListener func(event GraphEvent)

// Node 表示有向无环图中的一个节点，封装一个 agent 及其依赖关系
type Node struct {
	Name        string
	Description string

	agent    agent.Agent
	next     []*Node
	inDegree int
}

func NewNode(name string, des string, ag agent.Agent) *Node {
	return &Node{
		Name:        name,
		Description: des,
		agent:       ag,
	}
}

func (n *Node) AddNext(next *Node) error {
	if next == nil {
		return fmt.Errorf("cannot add nil as next node")
	}
	if n == next {
		return fmt.Errorf("cannot add self as next node")
	}
	for _, val := range n.next {
		if next == val {
			return fmt.Errorf("edge already exists from %q to %q", n.Name, next.Name)
		}
	}
	n.next = append(n.next, next)
	next.inDegree++
	return nil
}

type Graph struct {
	Name        string
	Description string

	root      *Node
	nodes     map[string]*Node
	mu        sync.RWMutex
	listeners []GraphEventListener
}

func NewGraph(name string, des string) *Graph {
	return &Graph{
		Name:        name,
		Description: des,
		nodes:       make(map[string]*Node),
	}
}

// OnEvent 注册全局事件监听器
func (g *Graph) OnEvent(listener GraphEventListener) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.listeners = append(g.listeners, listener)
}

// fireEvent 触发事件，通知所有注册的监听器
func (g *Graph) fireEvent(event GraphEvent) {
	for _, listener := range g.listeners {
		listener(event)
	}
}

// AddNode 向图中添加一个节点。如果同名节点已存在则返回错误。
func (g *Graph) AddNode(node *Node) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if node == nil {
		return fmt.Errorf("cannot add nil node")
	}
	if _, ok := g.nodes[node.Name]; ok {
		return fmt.Errorf("node %q already exists", node.Name)
	}
	g.nodes[node.Name] = node
	return nil
}

// AddEdge 添加一条从 src 到 dst 的有向边。如果节点不存在或会形成环则返回错误。
func (g *Graph) AddEdge(src, dst string) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if src == dst {
		return fmt.Errorf("cannot add self-loop on %q", src)
	}
	srcNode, ok := g.nodes[src]
	if !ok {
		return fmt.Errorf("source node %q not found", src)
	}
	dstNode, ok := g.nodes[dst]
	if !ok {
		return fmt.Errorf("destination node %q not found", dst)
	}
	return srcNode.AddNext(dstNode)
}

func (g *Graph) SetRoot(name string) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	node, ok := g.nodes[name]
	if !ok {
		return fmt.Errorf("node %q not found", name)
	}
	g.root = node
	return nil
}

func (g *Graph) NodeCount() int {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return len(g.nodes)
}

// asChain 通过拓扑排序将图转换为线性节点序列。
// 仅遍历从 root 可达的节点，忽略孤立点和其他连通分量。
func (g *Graph) asChain() ([]*Node, error) {
	if len(g.nodes) == 0 {
		return nil, fmt.Errorf("graph has no nodes")
	}
	if g.root == nil {
		return nil, fmt.Errorf("root node not set, call SetRoot first")
	}

	// 从 root 出发 BFS 收集所有可达节点
	reachable := g.reachable()
	inDegree := make(map[string]int)
	for name, node := range reachable {
		inDegree[name] = node.inDegree
	}

	var q []*Node
	for name, deg := range inDegree {
		if deg == 0 {
			q = append(q, reachable[name])
		}
	}

	var sorted []*Node

	for len(q) > 0 {
		current := q[0]
		q = q[1:]
		sorted = append(sorted, current)

		for _, next := range current.next {
			if _, ok := reachable[next.Name]; !ok {
				continue
			}
			inDegree[next.Name]--
			if inDegree[next.Name] == 0 {
				q = append(q, next)
			}
		}
	}

	// 排序结果数量不等于可达节点总数，说明存在环
	if len(sorted) != len(reachable) {
		return nil, fmt.Errorf("graph contains a cycle: sorted %d nodes but reachable is %d",
			len(sorted), len(reachable))
	}

	return sorted, nil
}

func (g *Graph) reachable() map[string]*Node {
	g.mu.RLock()
	defer g.mu.RUnlock()
	if g.root == nil {
		return nil
	}
	reach := make(map[string]*Node)
	q := []*Node{g.root}
	reach[g.root.Name] = g.root
	for len(q) > 0 {
		cur := q[0]
		reach[cur.Name] = cur
		q = q[1:]
		for _, node := range cur.next {
			q = append(q, node)
		}
	}
	return reach
}

func (g *Graph) Run(ctx context.Context, session *memory.Session, input string) (string, error) {
	g.mu.RLock()
	if len(g.nodes) == 0 {
		g.mu.RUnlock()
		return "", fmt.Errorf("graph has no nodes")
	}
	g.mu.RUnlock()

	sorted, err := g.asChain()
	if err != nil {
		return "", fmt.Errorf("topological sort failed: %w", err)
	}

	// 存储每个节点的输出，用于传递给后继节点
	outputs := make(map[string]string)
	currentInput := input

	for i, node := range sorted {
		// 检查 context 是否已取消
		if ctx.Err() != nil {
			return "", fmt.Errorf("graph execution cancelled at node %q (index %d): %w",
				node.Name, i, ctx.Err())
		}

		// 触发节点开始事件
		g.fireEvent(GraphEvent{
			Type:      GraphNodeStarted,
			NodeName:  node.Name,
			Timestamp: time.Now(),
		})

		// 执行节点的 agent
		output, err := node.agent.Run(ctx, session, currentInput)
		if err != nil {
			g.fireEvent(GraphEvent{
				Type:      GraphNodeFailed,
				NodeName:  node.Name,
				Err:       err,
				Timestamp: time.Now(),
			})
			return "", fmt.Errorf("graph node %q (index %d) failed: %w",
				node.Name, i, err)
		}

		// 记录输出
		outputs[node.Name] = output
		currentInput = output

		// 触发节点完成事件
		g.fireEvent(GraphEvent{
			Type:      GraphNodeCompleted,
			NodeName:  node.Name,
			Output:    output,
			Timestamp: time.Now(),
		})
	}

	return currentInput, nil
}
