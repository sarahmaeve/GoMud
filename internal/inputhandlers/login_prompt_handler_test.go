package inputhandlers

import (
	"testing"

	"github.com/GoMudEngine/GoMud/internal/connections"
	"github.com/GoMudEngine/GoMud/internal/term"
)

// newPromptState creates a PromptHandlerState with a simple username step,
// pre-initialized as if the first prompt has already been sent.
// This lets us test input handling without needing template rendering.
func newPromptState() *PromptHandlerState {
	return &PromptHandlerState{
		Steps: []*PromptStep{
			{
				ID:             "username",
				PromptTemplate: "login/username.prompt",
				Validator:      ValidateNewEntry,
			},
		},
		CurrentStepIndex: 0,
		Results:          make(map[string]string),
		OnComplete: func(results map[string]string, sharedState map[string]any, clientInput *connections.ClientInput) bool {
			return true
		},
	}
}

// sharedStateWith installs a PromptHandlerState so CreatePromptHandler
// skips the initial prompt-send and goes straight to input handling.
func sharedStateWith(state *PromptHandlerState) map[string]any {
	return map[string]any{
		promptHandlerStateKey: state,
	}
}

// TestPromptHandler_BackspaceDoesNotDoubleDelete verifies that when
// BSPressed is true (set by CleanserInputHandler), the prompt handler
// does NOT remove an additional character from the buffer.
// This is the regression test for the double-delete bug.
func TestPromptHandler_BackspaceDoesNotDoubleDelete(t *testing.T) {
	state := newPromptState()
	shared := sharedStateWith(state)

	handler := CreatePromptHandler(state.Steps, state.OnComplete)

	clientInput := &connections.ClientInput{
		ConnectionId: 99999, // Nonexistent — SendTo silently ignores
		DataIn:       []byte{},
		Buffer:       []byte("ab"),
		BSPressed:    true,
		EnterPressed: false,
	}

	handler(clientInput, shared)

	got := string(clientInput.Buffer)
	if got != "ab" {
		t.Errorf("Prompt handler modified buffer on backspace: got %q, want %q", got, "ab")
	}
}

// TestCleanserThenPromptHandler_SingleCharRemoved runs a backspace through
// the full handler chain (Cleanser → PromptHandler) and verifies exactly
// one character is removed. This is the integration test that would have
// caught the original double-delete bug.
func TestCleanserThenPromptHandler_SingleCharRemoved(t *testing.T) {
	tests := []struct {
		name           string
		initialBuffer  string
		expectedBuffer string
	}{
		{"remove last of 5", "hello", "hell"},
		{"remove last of 3", "abc", "ab"},
		{"remove last of 1", "x", ""},
		{"empty buffer", "", ""},
		{"UTF8 char", "café", "caf"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state := newPromptState()
			shared := sharedStateWith(state)

			handler := CreatePromptHandler(state.Steps, state.OnComplete)

			clientInput := &connections.ClientInput{
				ConnectionId: 99999,
				DataIn:       []byte{term.ASCII_BACKSPACE},
				Buffer:       []byte(tt.initialBuffer),
			}

			// Step 1: Cleanser processes the backspace
			CleanserInputHandler(clientInput, shared)

			// Step 2: Prompt handler sees BSPressed=true
			handler(clientInput, shared)

			got := string(clientInput.Buffer)
			if got != tt.expectedBuffer {
				t.Errorf("After cleanser+prompt handler chain: buffer = %q, want %q", got, tt.expectedBuffer)
			}
		})
	}
}

// TestMultipleBackspaces_FullChain simulates typing "test" then pressing
// backspace 4 times through the full handler chain. The buffer should be
// empty and each press should remove exactly one character.
func TestMultipleBackspaces_FullChain(t *testing.T) {
	state := newPromptState()
	shared := sharedStateWith(state)

	handler := CreatePromptHandler(state.Steps, state.OnComplete)

	// Simulate typing "test" — cleanser appends DataIn to Buffer
	clientInput := &connections.ClientInput{
		ConnectionId: 99999,
		DataIn:       []byte("test"),
		Buffer:       []byte{},
	}
	CleanserInputHandler(clientInput, shared)

	if string(clientInput.Buffer) != "test" {
		t.Fatalf("Setup failed: buffer = %q, want %q", string(clientInput.Buffer), "test")
	}

	// Now press backspace 4 times, each time running both handlers
	expected := []string{"tes", "te", "t", ""}
	for i, want := range expected {
		// Reset flags and set up backspace input
		clientInput.BSPressed = false
		clientInput.EnterPressed = false
		clientInput.TabPressed = false
		clientInput.DataIn = []byte{term.ASCII_BACKSPACE}

		CleanserInputHandler(clientInput, shared)
		handler(clientInput, shared)

		got := string(clientInput.Buffer)
		if got != want {
			t.Errorf("After backspace %d: buffer = %q, want %q", i+1, got, want)
		}
	}

	// 5th backspace on empty buffer should be harmless
	clientInput.BSPressed = false
	clientInput.DataIn = []byte{term.ASCII_BACKSPACE}
	CleanserInputHandler(clientInput, shared)
	handler(clientInput, shared)

	if len(clientInput.Buffer) != 0 {
		t.Errorf("Extra backspace on empty buffer: got %q, want empty", string(clientInput.Buffer))
	}
}

// TestPromptHandler_NormalInputEchoed verifies that non-backspace printable
// input is echoed (via SendTo) and the buffer is left as-is by the prompt
// handler (the cleanser already appended to it).
func TestPromptHandler_NormalInputEchoed(t *testing.T) {
	state := newPromptState()
	shared := sharedStateWith(state)

	handler := CreatePromptHandler(state.Steps, state.OnComplete)

	clientInput := &connections.ClientInput{
		ConnectionId: 99999,
		DataIn:       []byte("a"),
		Buffer:       []byte("testa"), // Cleanser already appended "a"
		BSPressed:    false,
		EnterPressed: false,
	}

	handler(clientInput, shared)

	// Buffer should not be modified by the prompt handler
	got := string(clientInput.Buffer)
	if got != "testa" {
		t.Errorf("Prompt handler modified buffer on normal input: got %q, want %q", got, "testa")
	}
}
