package utils

import (
	"testing"
)

type testStruct struct {
	Name  string `json:"name"`
	Value int    `json:"value"`
}

func TestParseJSON_PureJSON(t *testing.T) {
	var s testStruct
	err := ParseJSON(`{"name":"foo","value":42}`, &s)
	if err != nil {
		t.Fatal(err)
	}
	if s.Name != "foo" || s.Value != 42 {
		t.Fatalf("unexpected: %+v", s)
	}
}

func TestParseJSON_CodeBlock(t *testing.T) {
	input := "some text\n```json\n{\"name\":\"bar\",\"value\":7}\n```\nmore text"
	var s testStruct
	err := ParseJSON(input, &s)
	if err != nil {
		t.Fatal(err)
	}
	if s.Name != "bar" || s.Value != 7 {
		t.Fatalf("unexpected: %+v", s)
	}
}

func TestParseJSON_CodeBlockNoLabel(t *testing.T) {
	input := "analysis:\n```\n{\"name\":\"baz\",\"value\":9}\n```"
	var s testStruct
	err := ParseJSON(input, &s)
	if err != nil {
		t.Fatal(err)
	}
	if s.Name != "baz" || s.Value != 9 {
		t.Fatalf("unexpected: %+v", s)
	}
}

func TestParseJSON_MixedText(t *testing.T) {
	input := "Here is the result:\n{\"name\":\"hello\",\"value\":1}\nDone."
	var s testStruct
	err := ParseJSON(input, &s)
	if err != nil {
		t.Fatal(err)
	}
	if s.Name != "hello" || s.Value != 1 {
		t.Fatalf("unexpected: %+v", s)
	}
}

func TestParseJSON_WithBracesInString(t *testing.T) {
	input := `The answer is {"name":"a{b}c","value":0} end`
	var s testStruct
	err := ParseJSON(input, &s)
	if err != nil {
		t.Fatal(err)
	}
	if s.Name != "a{b}c" {
		t.Fatalf("unexpected: %+v", s)
	}
}

func TestParseJSON_Array(t *testing.T) {
	input := "results:\n[{\"name\":\"x\",\"value\":1},{\"name\":\"y\",\"value\":2}]"
	var arr []testStruct
	err := ParseJSON(input, &arr)
	if err != nil {
		t.Fatal(err)
	}
	if len(arr) != 2 || arr[0].Name != "x" || arr[1].Name != "y" {
		t.Fatalf("unexpected: %+v", arr)
	}
}

func TestParseJSON_Whitespace(t *testing.T) {
	input := "  \n  {\"name\":\"ws\",\"value\":0}  \n  "
	var s testStruct
	err := ParseJSON(input, &s)
	if err != nil {
		t.Fatal(err)
	}
	if s.Name != "ws" {
		t.Fatalf("unexpected: %+v", s)
	}
}
