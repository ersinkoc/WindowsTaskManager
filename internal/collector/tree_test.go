//go:build windows

package collector

import (
	"testing"

	"github.com/ersinkoc/WindowsTaskManager/internal/metrics"
)

func TestBuildProcessTreeEmpty(t *testing.T) {
	roots := BuildProcessTree(nil)
	if len(roots) != 0 {
		t.Fatalf("expected 0 roots, got %d", len(roots))
	}
	roots = BuildProcessTree([]metrics.ProcessInfo{})
	if len(roots) != 0 {
		t.Fatalf("expected 0 roots, got %d", len(roots))
	}
}

func TestBuildProcessTreeSingleRoot(t *testing.T) {
	procs := []metrics.ProcessInfo{
		{PID: 1, Name: "root"},
	}
	roots := BuildProcessTree(procs)
	if len(roots) != 1 {
		t.Fatalf("expected 1 root, got %d", len(roots))
	}
	if roots[0].Process.PID != 1 {
		t.Fatalf("root PID=%d", roots[0].Process.PID)
	}
	if roots[0].Depth != 0 {
		t.Fatalf("root depth=%d", roots[0].Depth)
	}
}

func TestBuildProcessTreeWithChildren(t *testing.T) {
	procs := []metrics.ProcessInfo{
		{PID: 1, Name: "root", CPUPercent: 50},
		{PID: 2, ParentPID: 1, Name: "child1", CPUPercent: 30},
		{PID: 3, ParentPID: 1, Name: "child2", CPUPercent: 70},
	}
	roots := BuildProcessTree(procs)
	if len(roots) != 1 {
		t.Fatalf("expected 1 root, got %d", len(roots))
	}
	if len(roots[0].Children) != 2 {
		t.Fatalf("expected 2 children, got %d", len(roots[0].Children))
	}
	// Children sorted by CPU desc.
	if roots[0].Children[0].Process.PID != 3 {
		t.Fatalf("expected child3 first (CPU 70), got %d", roots[0].Children[0].Process.PID)
	}
	if roots[0].Children[1].Process.PID != 2 {
		t.Fatalf("expected child2 second (CPU 30), got %d", roots[0].Children[1].Process.PID)
	}
	if roots[0].Children[0].Depth != 1 {
		t.Fatalf("child depth=%d", roots[0].Children[0].Depth)
	}
}

func TestBuildProcessTreeParentZeroIsRoot(t *testing.T) {
	procs := []metrics.ProcessInfo{
		{PID: 5, ParentPID: 0, Name: "orphan"},
	}
	roots := BuildProcessTree(procs)
	if len(roots) != 1 {
		t.Fatalf("expected 1 root, got %d", len(roots))
	}
	if roots[0].Process.PID != 5 {
		t.Fatalf("root PID=%d", roots[0].Process.PID)
	}
}

func TestBuildProcessTreeDetectsOrphanByCreateTime(t *testing.T) {
	// Setup: parent is itself a root (parent_pid=0), and one of its children
	// was created BEFORE the parent — that signals PID reuse, mark as orphan.
	procs := []metrics.ProcessInfo{
		{PID: 100, ParentPID: 0, Name: "parent", CreateTime: 200},
		{PID: 200, ParentPID: 100, Name: "child", CreateTime: 100},
	}
	roots := BuildProcessTree(procs)
	if len(roots) != 2 {
		t.Fatalf("expected 2 roots, got %d", len(roots))
	}
	var orphan *metrics.ProcessNode
	for _, r := range roots {
		if r.IsOrphan {
			orphan = r
			break
		}
	}
	if orphan == nil {
		t.Fatal("expected one root to be flagged orphan")
	}
	if orphan.Process.PID != 200 {
		t.Fatalf("orphan PID=%d want 200", orphan.Process.PID)
	}
}

func TestBuildProcessTreeMultipleRootsSorted(t *testing.T) {
	procs := []metrics.ProcessInfo{
		{PID: 1, Name: "low", CPUPercent: 10},
		{PID: 2, Name: "high", CPUPercent: 90},
		{PID: 3, Name: "mid", CPUPercent: 50},
	}
	roots := BuildProcessTree(procs)
	if len(roots) != 3 {
		t.Fatalf("expected 3 roots, got %d", len(roots))
	}
	if roots[0].Process.PID != 2 || roots[1].Process.PID != 3 || roots[2].Process.PID != 1 {
		t.Fatalf("roots not sorted by CPU desc: %+v", roots)
	}
}

func TestAssignDepthRecursive(t *testing.T) {
	root := &metrics.ProcessNode{Process: metrics.ProcessInfo{PID: 1}}
	mid := &metrics.ProcessNode{Process: metrics.ProcessInfo{PID: 2}}
	leaf := &metrics.ProcessNode{Process: metrics.ProcessInfo{PID: 3}}
	mid.Children = []*metrics.ProcessNode{leaf}
	root.Children = []*metrics.ProcessNode{mid}

	assignDepth(root, 0)
	if root.Depth != 0 {
		t.Fatalf("root depth=%d", root.Depth)
	}
	if mid.Depth != 1 {
		t.Fatalf("mid depth=%d", mid.Depth)
	}
	if leaf.Depth != 2 {
		t.Fatalf("leaf depth=%d", leaf.Depth)
	}
}

func TestAssignDepthOnLeafNode(t *testing.T) {
	leaf := &metrics.ProcessNode{Process: metrics.ProcessInfo{PID: 1}}
	assignDepth(leaf, 5)
	if leaf.Depth != 5 {
		t.Fatalf("depth=%d", leaf.Depth)
	}
}

func TestBuildProcessTreeNestedDepth(t *testing.T) {
	procs := []metrics.ProcessInfo{
		{PID: 1, Name: "root"},
		{PID: 2, ParentPID: 1, Name: "mid"},
		{PID: 3, ParentPID: 2, Name: "leaf"},
	}
	roots := BuildProcessTree(procs)
	if len(roots) != 1 {
		t.Fatalf("expected 1 root, got %d", len(roots))
	}
	if len(roots[0].Children) != 1 {
		t.Fatalf("expected 1 child of root")
	}
	if len(roots[0].Children[0].Children) != 1 {
		t.Fatalf("expected 1 grandchild")
	}
	if roots[0].Children[0].Children[0].Depth != 2 {
		t.Fatalf("grandchild depth=%d", roots[0].Children[0].Children[0].Depth)
	}
}
