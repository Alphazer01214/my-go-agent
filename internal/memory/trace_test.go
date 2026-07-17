package memory

import (
	"my-go-agent/internal/llm"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// ==================== Diff.Reverse Tests ====================

func TestDiffReverse_FileModify(t *testing.T) {
	d := &Diff{
		eventId:  "evt1",
		msgAdded: 3,
		fileChanges: []FileChange{
			{Path: "a", Op: FileModify, Before: "old", After: "new"},
		},
	}

	r := d.Reverse()
	if r == nil {
		t.Fatal("expected non-nil reverse")
	}
	if r.msgAdded != 3 {
		t.Errorf("msgAdded = %d, want 3", r.msgAdded)
	}
	if len(r.fileChanges) != 1 {
		t.Fatalf("fileChanges len = %d, want 1", len(r.fileChanges))
	}
	fc := r.fileChanges[0]
	if fc.Path != "a" || fc.Op != FileModify {
		t.Errorf("fileChange = %+v, want Path=a Op=modify", fc)
	}
	if fc.Before != "new" || fc.After != "old" {
		t.Errorf("Before/After swapped: Before=%q After=%q, want Before=new After=old", fc.Before, fc.After)
	}
}

func TestDiffReverse_FileCreate(t *testing.T) {
	d := &Diff{
		eventId:  "evt2",
		msgAdded: 2,
		fileChanges: []FileChange{
			{Path: "new.go", Op: FileCreate, Before: "", After: "content"},
		},
	}

	r := d.Reverse()
	if r == nil {
		t.Fatal("expected non-nil reverse")
	}
	fc := r.fileChanges[0]
	if fc.Op != FileDelete {
		t.Errorf("reverse of create should be delete, got %s", fc.Op)
	}
	if fc.Before != "content" {
		t.Errorf("Before = %q, want content (for potential recovery)", fc.Before)
	}
}

func TestDiffReverse_FileDelete(t *testing.T) {
	d := &Diff{
		eventId:  "evt3",
		msgAdded: 1,
		fileChanges: []FileChange{
			{Path: "del.go", Op: FileDelete, Before: "old_content", After: ""},
		},
	}

	r := d.Reverse()
	if r != nil {
		t.Error("reverse of delete should be nil (irreversible)")
	}
}

func TestDiffReverse_MultipleChanges(t *testing.T) {
	d := &Diff{
		eventId:  "evt4",
		msgAdded: 4,
		fileChanges: []FileChange{
			{Path: "a", Op: FileModify, Before: "A", After: "B"},
			{Path: "b.go", Op: FileCreate, Before: "", After: "new_b"},
			{Path: "c.go", Op: FileModify, Before: "C", After: "D"},
		},
	}

	r := d.Reverse()
	if r == nil {
		t.Fatal("expected non-nil reverse")
	}
	if len(r.fileChanges) != 3 {
		t.Fatalf("fileChanges len = %d, want 3", len(r.fileChanges))
	}
	// 反向应该是逆序处理后的结果（但顺序保持，回滚时逆序遍历）
	if r.fileChanges[0].Before != "B" || r.fileChanges[0].After != "A" {
		t.Errorf("first change not reversed correctly: %+v", r.fileChanges[0])
	}
	if r.fileChanges[1].Op != FileDelete {
		t.Errorf("second change (create) should reverse to delete, got %s", r.fileChanges[1].Op)
	}
}

func TestDiffReverse_Empty(t *testing.T) {
	d := &Diff{eventId: "evt5", msgAdded: 0}
	r := d.Reverse()
	if r == nil {
		t.Fatal("expected non-nil reverse for empty diff")
	}
	if len(r.fileChanges) != 0 {
		t.Errorf("fileChanges len = %d, want 0", len(r.fileChanges))
	}
}

// ==================== Record Method Tests ====================

func TestAddUserEvent(t *testing.T) {
	tr := NewTrace("test")
	tr.AddUserEvent(0, "hello world", nil)

	events := tr.Events()
	if len(events) != 1 {
		t.Fatalf("events len = %d, want 1", len(events))
	}
	e := events[0]
	if e.Type != EventUser {
		t.Errorf("type = %s, want user", e.Type)
	}
	if e.Subject != "hello world" {
		t.Errorf("subject = %q, want hello world", e.Subject)
	}
	if !e.Recoverable {
		t.Error("user event should be recoverable (message-only)")
	}
}

func TestAddToolCallEvent_WithFileChanges(t *testing.T) {
	tr := NewTrace("test")
	tr.AddToolCallEvent(1,
		[]ToolCall{{Name: "write_file", Args: `{"path":"a"}`, Result: "ok"}},
		[]FileChange{{Path: "a", Op: FileCreate, Before: "", After: "content"}},
		3, // 1 assistant + 2 tool results
		[]string{"write_file"},
		nil,
	)

	events := tr.Events()
	if len(events) != 1 {
		t.Fatalf("events len = %d, want 1", len(events))
	}
	e := events[0]
	if e.Type != EventToolCall {
		t.Errorf("type = %s, want tool_call", e.Type)
	}
	if !e.Recoverable {
		t.Error("tool call with file create should be recoverable")
	}
	if e.diff == nil {
		t.Fatal("diff should not be nil")
	}
	if e.diff.msgAdded != 3 {
		t.Errorf("msgAdded = %d, want 3", e.diff.msgAdded)
	}
	if len(e.diff.fileChanges) != 1 {
		t.Fatalf("fileChanges len = %d, want 1", len(e.diff.fileChanges))
	}
}

func TestAddToolCallEvent_WithExecCommand(t *testing.T) {
	tr := NewTrace("test")
	tr.AddToolCallEvent(1,
		[]ToolCall{{Name: "exec_command", Args: `{"command":"ls"}`, Result: "ok"}},
		nil, // exec_command 无文件变更
		2,
		[]string{"exec_command"},
		nil,
	)

	events := tr.Events()
	e := events[0]
	if e.Recoverable {
		t.Error("exec_command should not be recoverable")
	}
}

func TestAddToolCallEvent_WithFileDelete(t *testing.T) {
	tr := NewTrace("test")
	tr.AddToolCallEvent(1,
		[]ToolCall{{Name: "exec_command", Args: `{"command":"rm f"}`, Result: "ok"}},
		[]FileChange{{Path: "f.go", Op: FileDelete, Before: "content", After: ""}},
		2,
		[]string{"exec_command"},
		nil,
	)

	events := tr.Events()
	e := events[0]
	if e.Recoverable {
		t.Error("file delete should not be recoverable")
	}
}

func TestAddToolCallEvent_ReadOnlyTools(t *testing.T) {
	// read_file、list_dir、glob、search_file 等只读工具应可回溯
	readOnlyTools := []string{"read_file", "list_dir", "glob", "search_file"}
	for _, toolName := range readOnlyTools {
		tr := NewTrace("test")
		tr.AddToolCallEvent(1,
			[]ToolCall{{Name: toolName, Args: "{}", Result: "content"}},
			nil, // 无文件变更
			2,
			[]string{toolName},
			nil,
		)
		events := tr.Events()
		if !events[0].Recoverable {
			t.Errorf("read-only tool %s should be recoverable", toolName)
		}
	}
}

func TestAddSandboxEvent(t *testing.T) {
	tr := NewTrace("test")
	tr.AddSandboxEvent(1, "exec_command", "blocked by sandbox")

	events := tr.Events()
	e := events[0]
	if e.Type != EventSandbox {
		t.Errorf("type = %s, want sandbox", e.Type)
	}
	if e.Recoverable {
		t.Error("sandbox event should not be recoverable")
	}
}

func TestAddRestoreEvent(t *testing.T) {
	tr := NewTrace("test")
	tr.AddRestoreEvent("evt1", "Restored to evt1")

	events := tr.Events()
	e := events[0]
	if e.Type != EventRestore {
		t.Errorf("type = %s, want restore", e.Type)
	}
	if e.Recoverable {
		t.Error("restore event should not be recoverable")
	}
}

// ==================== Restore Tests ====================

// helper: 创建带消息的 session
func createTestSession() *Session {
	return NewSession(nil, "test")
}

// helper: 创建临时文件
func createTempFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestRestore_Basic(t *testing.T) {
	dir := t.TempDir()
	path := createTempFile(t, dir, "test.go", "original")

	session := createTestSession()
	// 添加一些消息
	session.AddMessage(llm.UserMessage("do something"))
	session.AddMessage(llm.AssistantMessageWithToolCall("let me edit", nil))
	session.AddMessage(llm.ToolMessage("call1", "ok"))
	session.AddMessage(llm.AssistantMessage("done"))

	trace := session.Trace()

	// Event 0: user
	trace.AddUserEvent(0, "do something", nil)

	// Event 1: tool call that modifies file
	trace.AddToolCallEvent(1,
		[]ToolCall{{Name: "edit_file", Args: "{}", Result: "ok"}},
		[]FileChange{{Path: path, Op: FileModify, Before: "original", After: "modified"}},
		3, // assistant + tool result
		[]string{"edit_file"},
		nil,
	)
	// 实际修改文件
	os.WriteFile(path, []byte("modified"), 0644)

	// Event 2: response
	trace.AddResponseEvent(1, "done", nil)

	// 验证文件被修改
	data, _ := os.ReadFile(path)
	if string(data) != "modified" {
		t.Fatalf("file should be modified, got %q", string(data))
	}

	// 恢复到 Event 0
	events := trace.Events()
	result, err := trace.Restore(events[0].Id, session)
	if err != nil {
		t.Fatalf("restore failed: %v", err)
	}
	if result.Skipped != 0 {
		t.Errorf("skipped = %d, want 0", result.Skipped)
	}

	// 验证文件恢复
	data, _ = os.ReadFile(path)
	if string(data) != "original" {
		t.Errorf("file should be restored to original, got %q", string(data))
	}

	// 验证 trace 被截断 + restore 事件
	events = trace.Events()
	if len(events) != 2 { // user event + restore event
		t.Errorf("events len = %d, want 2", len(events))
	}
	if events[1].Type != EventRestore {
		t.Errorf("last event type = %s, want restore", events[1].Type)
	}
}

func TestRestore_FileCreate(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "new.go")

	session := createTestSession()
	session.AddMessage(llm.UserMessage("create file"))
	session.AddMessage(llm.AssistantMessageWithToolCall("creating", nil))
	session.AddMessage(llm.ToolMessage("call1", "ok"))

	trace := session.Trace()
	trace.AddUserEvent(0, "create file", nil)
	trace.AddToolCallEvent(1,
		[]ToolCall{{Name: "write_file", Args: "{}", Result: "ok"}},
		[]FileChange{{Path: path, Op: FileCreate, Before: "", After: "new content"}},
		2,
		[]string{"write_file"},
		nil,
	)
	// 实际创建文件
	os.WriteFile(path, []byte("new content"), 0644)

	// 验证文件存在
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Fatal("file should exist")
	}

	// 恢复
	events := trace.Events()
	result, err := trace.Restore(events[0].Id, session)
	if err != nil {
		t.Fatalf("restore failed: %v", err)
	}
	if result.FilesReverted != 1 {
		t.Errorf("files reverted = %d, want 1", result.FilesReverted)
	}

	// 验证文件被删除
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("file should be deleted after restore")
	}
}

func TestRestore_MultipleEvents_ReverseOrder(t *testing.T) {
	dir := t.TempDir()
	path := createTempFile(t, dir, "multi.go", "V0")

	session := createTestSession()
	session.AddMessage(llm.UserMessage("edit"))
	session.AddMessage(llm.AssistantMessageWithToolCall("step1", nil))
	session.AddMessage(llm.ToolMessage("c1", "ok"))
	session.AddMessage(llm.AssistantMessageWithToolCall("step2", nil))
	session.AddMessage(llm.ToolMessage("c2", "ok"))
	session.AddMessage(llm.AssistantMessage("done"))

	trace := session.Trace()
	trace.AddUserEvent(0, "edit", nil)

	// Event 1: V0 → V1
	trace.AddToolCallEvent(1,
		[]ToolCall{{Name: "edit_file"}},
		[]FileChange{{Path: path, Op: FileModify, Before: "V0", After: "V1"}},
		3,
		[]string{"edit_file"},
		nil,
	)
	os.WriteFile(path, []byte("V1"), 0644)

	// Event 2: V1 → V2
	trace.AddToolCallEvent(2,
		[]ToolCall{{Name: "edit_file"}},
		[]FileChange{{Path: path, Op: FileModify, Before: "V1", After: "V2"}},
		3,
		[]string{"edit_file"},
		nil,
	)
	os.WriteFile(path, []byte("V2"), 0644)

	// Event 3: response
	trace.AddResponseEvent(2, "done", nil)

	// 恢复到 Event 0
	events := trace.Events()
	result, err := trace.Restore(events[0].Id, session)
	if err != nil {
		t.Fatalf("restore failed: %v", err)
	}
	if result.FilesReverted != 2 {
		t.Errorf("files reverted = %d, want 2", result.FilesReverted)
	}

	// 验证文件恢复到 V0（逆序回滚 V2→V1→V0）
	data, _ := os.ReadFile(path)
	if string(data) != "V0" {
		t.Errorf("file = %q, want V0", string(data))
	}
}

func TestRestore_HardRestore_IrreversibleEvent(t *testing.T) {
	dir := t.TempDir()
	path := createTempFile(t, dir, "a", "old")

	session := createTestSession()
	session.AddMessage(llm.UserMessage("run"))
	session.AddMessage(llm.AssistantMessageWithToolCall("exec", nil))
	session.AddMessage(llm.ToolMessage("c1", "output"))
	session.AddMessage(llm.AssistantMessageWithToolCall("edit", nil))
	session.AddMessage(llm.ToolMessage("c2", "ok"))
	session.AddMessage(llm.AssistantMessage("done"))

	trace := session.Trace()
	trace.AddUserEvent(0, "run", nil)

	// Event 1: exec_command (不可恢复)
	trace.AddToolCallEvent(1,
		[]ToolCall{{Name: "exec_command", Args: `{"command":"ls"}`, Result: "output"}},
		nil, // 无文件变更
		2,
		[]string{"exec_command"},
		nil,
	)

	// Event 2: file edit (可恢复)
	trace.AddToolCallEvent(2,
		[]ToolCall{{Name: "edit_file"}},
		[]FileChange{{Path: path, Op: FileModify, Before: "old", After: "new"}},
		2,
		[]string{"edit_file"},
		nil,
	)
	os.WriteFile(path, []byte("new"), 0644)

	// 硬回溯：Event 1 不可逆但消息仍回滚，Event 2 文件+消息都回滚
	events := trace.Events()
	result, err := trace.Restore(events[0].Id, session)
	if err != nil {
		t.Fatalf("hard restore should succeed, got error: %v", err)
	}
	if result.Skipped != 1 {
		t.Errorf("skipped = %d, want 1 (exec_command)", result.Skipped)
	}
	if result.RolledBack != 1 {
		t.Errorf("rolled back = %d, want 1 (file edit)", result.RolledBack)
	}
	if len(result.SkippedEvents) != 1 {
		t.Errorf("skipped events len = %d, want 1", len(result.SkippedEvents))
	}

	// 验证文件被回滚（Event 2 的变更被撤销）
	data, _ := os.ReadFile(path)
	if string(data) != "old" {
		t.Errorf("file = %q, want old (Event 2 should be reverted)", string(data))
	}

	// 验证 restore 事件记录了跳过信息
	lastEvt := trace.Last()
	if lastEvt.Type != EventRestore {
		t.Errorf("last event type = %s, want restore", lastEvt.Type)
	}
	if !strings.Contains(lastEvt.Subject, "skipped=1") {
		t.Errorf("restore event should mention skipped=1, got: %s", lastEvt.Subject)
	}
}

func TestRestore_HardRestore_MixedReversibleAndNot(t *testing.T) {
	dir := t.TempDir()
	pathEdit := createTempFile(t, dir, "edit.go", "original")
	pathDelete := createTempFile(t, dir, "delete.go", "to_delete")

	session := createTestSession()
	session.AddMessage(llm.UserMessage("do stuff"))
	session.AddMessage(llm.AssistantMessageWithToolCall("step1", nil))
	session.AddMessage(llm.ToolMessage("c1", "ok"))
	session.AddMessage(llm.AssistantMessageWithToolCall("step2", nil))
	session.AddMessage(llm.ToolMessage("c2", "ok"))
	session.AddMessage(llm.AssistantMessageWithToolCall("step3", nil))
	session.AddMessage(llm.ToolMessage("c3", "ok"))
	session.AddMessage(llm.AssistantMessage("done"))

	trace := session.Trace()
	trace.AddUserEvent(0, "do stuff", nil)

	// Event 1: file edit (可恢复)
	trace.AddToolCallEvent(1,
		[]ToolCall{{Name: "edit_file"}},
		[]FileChange{{Path: pathEdit, Op: FileModify, Before: "original", After: "edited"}},
		2,
		[]string{"edit_file"},
		nil,
	)
	os.WriteFile(pathEdit, []byte("edited"), 0644)

	// Event 2: file delete (不可恢复 — FileDelete)
	trace.AddToolCallEvent(2,
		[]ToolCall{{Name: "exec_command"}},
		[]FileChange{{Path: pathDelete, Op: FileDelete, Before: "to_delete", After: ""}},
		2,
		[]string{"exec_command"},
		nil,
	)
	os.Remove(pathDelete)

	// Event 3: response
	trace.AddResponseEvent(3, "done", nil)

	// 硬回溯到 Event 0
	events := trace.Events()
	result, err := trace.Restore(events[0].Id, session)
	if err != nil {
		t.Fatalf("hard restore should succeed: %v", err)
	}

	// Event 1 被正常回滚（文件恢复）
	data, _ := os.ReadFile(pathEdit)
	if string(data) != "original" {
		t.Errorf("edit.go = %q, want original", string(data))
	}

	// Event 2 被跳过（FileDelete 不可逆，文件无法恢复）
	if _, err := os.Stat(pathDelete); !os.IsNotExist(err) {
		t.Error("delete.go should still be deleted (irreversible)")
	}

	// 结果统计
	if result.Skipped != 1 {
		t.Errorf("skipped = %d, want 1", result.Skipped)
	}
	if result.RolledBack != 2 { // Event 1 + Event 3
		t.Errorf("rolled back = %d, want 2", result.RolledBack)
	}
	if result.FilesReverted != 1 {
		t.Errorf("files reverted = %d, want 1", result.FilesReverted)
	}
}

func TestRestore_DirCreate(t *testing.T) {
	dir := t.TempDir()
	newDir := filepath.Join(dir, "newpkg")

	session := createTestSession()
	session.AddMessage(llm.UserMessage("create dir"))
	session.AddMessage(llm.AssistantMessageWithToolCall("mkdir", nil))
	session.AddMessage(llm.ToolMessage("c1", "ok"))

	trace := session.Trace()
	trace.AddUserEvent(0, "create dir", nil)
	trace.AddToolCallEvent(1,
		[]ToolCall{{Name: "make_dir", Args: `{"path":"newpkg"}`, Result: "ok"}},
		[]FileChange{{Path: newDir, Op: DirCreate, Before: "", After: newDir}},
		2,
		[]string{"make_dir"},
		nil,
	)
	// 实际创建目录
	os.Mkdir(newDir, 0755)

	// 验证目录存在
	if _, err := os.Stat(newDir); os.IsNotExist(err) {
		t.Fatal("dir should exist")
	}

	// 恢复 — 空目录应被删除
	events := trace.Events()
	result, err := trace.Restore(events[0].Id, session)
	if err != nil {
		t.Fatalf("restore failed: %v", err)
	}
	if result.FilesReverted != 1 {
		t.Errorf("files reverted = %d, want 1", result.FilesReverted)
	}

	// 验证目录被删除
	if _, err := os.Stat(newDir); !os.IsNotExist(err) {
		t.Error("empty dir should be deleted after restore")
	}
}

func TestRestore_DirCreate_ThenFileCreate(t *testing.T) {
	dir := t.TempDir()
	newDir := filepath.Join(dir, "newpkg")
	os.Mkdir(newDir, 0755)
	os.WriteFile(filepath.Join(newDir, "file.go"), []byte("code"), 0644)

	session := createTestSession()
	session.AddMessage(llm.UserMessage("mkdir + write"))
	session.AddMessage(llm.AssistantMessageWithToolCall("mkdir", nil))
	session.AddMessage(llm.ToolMessage("c1", "ok"))
	session.AddMessage(llm.AssistantMessageWithToolCall("write", nil))
	session.AddMessage(llm.ToolMessage("c2", "ok"))

	trace := session.Trace()
	trace.AddUserEvent(0, "mkdir + write", nil)
	// Event 1: 创建目录
	trace.AddToolCallEvent(1,
		[]ToolCall{{Name: "make_dir"}},
		[]FileChange{{Path: newDir, Op: DirCreate, Before: "", After: newDir}},
		2,
		[]string{"make_dir"},
		nil,
	)
	// Event 2: 在目录中写文件
	trace.AddToolCallEvent(2,
		[]ToolCall{{Name: "write_file"}},
		[]FileChange{{Path: filepath.Join(newDir, "file.go"), Op: FileCreate, Before: "", After: "code"}},
		2,
		[]string{"write_file"},
		nil,
	)

	// 恢复到 Event 0 — 逆序：先删文件（目录变空），再删目录
	events := trace.Events()
	result, err := trace.Restore(events[0].Id, session)
	if err != nil {
		t.Fatalf("restore failed: %v", err)
	}
	if result.FilesReverted != 2 { // file.go + 目录
		t.Errorf("files reverted = %d, want 2", result.FilesReverted)
	}

	// 目录应该被删除（文件先被回滚，目录变空后可删）
	if _, err := os.Stat(newDir); !os.IsNotExist(err) {
		t.Error("dir should be deleted after restore (file was removed first)")
	}
}

func TestRestore_DirCreate_WithExternalFile(t *testing.T) {
	dir := t.TempDir()
	newDir := filepath.Join(dir, "newpkg")
	os.Mkdir(newDir, 0755)
	// 目录中有一个不在 trace 中的外部文件
	os.WriteFile(filepath.Join(newDir, "external.txt"), []byte("data"), 0644)

	session := createTestSession()
	session.AddMessage(llm.UserMessage("mkdir"))
	session.AddMessage(llm.AssistantMessageWithToolCall("mkdir", nil))
	session.AddMessage(llm.ToolMessage("c1", "ok"))

	trace := session.Trace()
	trace.AddUserEvent(0, "mkdir", nil)
	// Event 1: 创建目录（trace 中没有记录 external.txt）
	trace.AddToolCallEvent(1,
		[]ToolCall{{Name: "make_dir"}},
		[]FileChange{{Path: newDir, Op: DirCreate, Before: "", After: newDir}},
		2,
		[]string{"make_dir"},
		nil,
	)

	// 恢复 — 目录非空（有外部文件），DirCreate 回滚静默跳过
	events := trace.Events()
	_, err := trace.Restore(events[0].Id, session)
	if err != nil {
		t.Fatalf("restore failed: %v", err)
	}

	// 目录应该还在（非空，无法删除）
	if _, err := os.Stat(newDir); os.IsNotExist(err) {
		t.Error("non-empty dir with external file should still exist after restore")
	}
}

func TestRestore_AlreadyAtTarget(t *testing.T) {
	trace := NewTrace("test")
	trace.AddUserEvent(0, "hello", nil)

	events := trace.Events()
	_, err := trace.Restore(events[0].Id, nil)
	if err == nil {
		t.Error("restore should fail when already at target")
	}
}

func TestRestore_EventNotFound(t *testing.T) {
	trace := NewTrace("test")
	trace.AddUserEvent(0, "hello", nil)

	_, err := trace.Restore("nonexistent", nil)
	if err == nil {
		t.Error("restore should fail for nonexistent event")
	}
}

func TestCanRestore(t *testing.T) {
	trace := NewTrace("test")
	trace.AddUserEvent(0, "hello", nil)
	trace.AddToolCallEvent(1,
		[]ToolCall{{Name: "edit_file"}},
		[]FileChange{{Path: "a", Op: FileModify, Before: "old", After: "new"}},
		2,
		[]string{"edit_file"},
		nil,
	)
	trace.AddResponseEvent(1, "done", nil)

	events := trace.Events()

	// 可以恢复到 user event（无不可逆事件）
	ok, willSkip, reason := trace.CanRestore(events[0].Id)
	if !ok {
		t.Errorf("should be able to restore to user event, reason: %s", reason)
	}
	if willSkip {
		t.Error("should not skip when all events are reversible")
	}

	// 不能恢复到最后一个事件（没有后续事件需要回滚）
	ok, _, reason = trace.CanRestore(events[2].Id)
	if ok {
		t.Errorf("should not be able to restore to last event, reason: %s", reason)
	}

	// 不存在的事件
	ok, _, reason = trace.CanRestore("nonexistent")
	if ok {
		t.Error("should not be able to restore to nonexistent event")
	}
}

func TestCanRestore_Irreversible(t *testing.T) {
	trace := NewTrace("test")
	trace.AddUserEvent(0, "run", nil)
	trace.AddToolCallEvent(1,
		[]ToolCall{{Name: "exec_command"}},
		nil,
		2,
		[]string{"exec_command"},
		nil,
	)
	trace.AddResponseEvent(1, "done", nil)

	events := trace.Events()
	ok, willSkip, reason := trace.CanRestore(events[0].Id)
	if !ok {
		t.Errorf("hard restore should be possible, reason: %s", reason)
	}
	if !willSkip {
		t.Error("willSkip should be true when exec_command is in the path")
	}
}

// ==================== Message Rollback Tests ====================

func TestRestore_MessageRollback(t *testing.T) {
	session := createTestSession()

	// 添加 system + user + assistant(tool_calls) + tool + response
	session.AddMessage(llm.SystemMessage("sys"))
	session.AddMessage(llm.UserMessage("do it"))
	session.AddMessage(llm.AssistantMessageWithToolCall("thinking", nil))
	session.AddMessage(llm.ToolMessage("c1", "result"))
	session.AddMessage(llm.AssistantMessage("done"))

	initialCount := session.MessageCount()

	trace := session.Trace()
	trace.AddUserEvent(0, "do it", nil)
	trace.AddToolCallEvent(1,
		[]ToolCall{{Name: "edit_file"}},
		[]FileChange{{Path: "a", Op: FileModify, Before: "old", After: "new"}},
		3,
		[]string{"edit_file"},
		nil,
	)
	trace.AddResponseEvent(1, "done", nil)

	// 恢复到 user event — 应该移除 tool call 和 response 的消息
	events := trace.Events()
	_, err := trace.Restore(events[0].Id, session)
	if err != nil {
		t.Fatalf("restore failed: %v", err)
	}

	// 消息数应该减少
	finalCount := session.MessageCount()
	if finalCount >= initialCount {
		t.Errorf("message count should decrease: initial=%d final=%d", initialCount, finalCount)
	}
}

// ==================== Format Tests ====================

func TestFormat_Empty(t *testing.T) {
	trace := NewTrace("test")
	s := trace.Format()
	if s != "(no trace events)" {
		t.Errorf("Format() = %q, want '(no trace events)'", s)
	}
}

func TestFormat_WithEvents(t *testing.T) {
	trace := NewTrace("test")
	trace.AddUserEvent(0, "hello", nil)
	trace.AddToolCallEvent(1,
		[]ToolCall{{Name: "write_file", Args: `{"path":"a"}`, Result: "ok"}},
		[]FileChange{{Path: "a", Op: FileCreate, Before: "", After: "content"}},
		2,
		[]string{"write_file"},
		nil,
	)
	trace.AddResponseEvent(1, "done", nil)

	s := trace.Format()
	if s == "" {
		t.Error("Format() should not be empty")
	}
	// 检查包含关键信息
	if !contains(s, "Trace: test") {
		t.Error("Format() should contain agent name")
	}
	if !contains(s, "user") {
		t.Error("Format() should contain user event")
	}
	if !contains(s, "tool_call") {
		t.Error("Format() should contain tool_call event")
	}
	if !contains(s, "response") {
		t.Error("Format() should contain response event")
	}
}

func TestFormatEvent_Restore(t *testing.T) {
	trace := NewTrace("test")
	e := Event{
		Id:          "evt_restore",
		Type:        EventRestore,
		Subject:     "Restored to evt1",
		Recoverable: false,
		Timestamp:   testTime(),
	}

	s := trace.FormatEvent(e)
	if !contains(s, "restore") {
		t.Error("FormatEvent(restore) should contain 'restore'")
	}
	if !contains(s, "irreversible") {
		t.Error("FormatEvent(restore) should contain 'irreversible'")
	}
}

// ==================== Tree Structure Tests ====================

func TestTraceTree_ParentChild(t *testing.T) {
	parent := NewTrace("parent")
	child := parent.NewChildTrace("child")

	if child.Parent() != parent {
		t.Error("child.Parent() should be parent")
	}
	if len(parent.Children()) != 1 {
		t.Fatalf("parent.Children() len = %d, want 1", len(parent.Children()))
	}
	if parent.Children()[0] != child {
		t.Error("parent.Children()[0] should be child")
	}
}

func TestAllEvents_Flattened(t *testing.T) {
	parent := NewTrace("parent")
	parent.AddUserEvent(0, "parent input", nil)

	child := parent.NewChildTrace("child")
	child.AddUserEvent(0, "child input", nil)
	child.AddResponseEvent(1, "child done", nil)

	all := parent.AllEvents()
	if len(all) != 3 {
		t.Errorf("AllEvents() len = %d, want 3", len(all))
	}
}

// ==================== Helpers ====================

func contains(s, substr string) bool {
	return strings.Contains(s, substr)
}

func testTime() time.Time {
	return time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
}
