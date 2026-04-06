package procedural

import (
	"fmt"
	"strings"
	"testing"
)

func TestGridMaze_Generate(t *testing.T) {
	tests := []struct {
		name  string
		xMax  int
		yMax  int
		seeds []string // Test with specific seeds for consistent results
	}{
		{
			name:  "Very Small maze 5x5",
			xMax:  5,
			yMax:  5,
			seeds: []string{"abcdef"},
		},
		{
			name:  "Small maze 8x8",
			xMax:  8,
			yMax:  8,
			seeds: []string{"one"},
		},
		{
			name:  "Medium maze 12x12",
			xMax:  12,
			yMax:  12,
			seeds: []string{"four"},
		},
		{
			name:  "Large maze 25x5",
			xMax:  25,
			yMax:  5,
			seeds: []string{"five"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for i, seed := range tt.seeds {
				t.Run(fmt.Sprintf("seed_%s", seed), func(t *testing.T) {
					// Create maze with deterministic seed
					maze := NewGridMaze()

					// Generate the maze
					rooms := maze.Generate2D(tt.xMax, tt.yMax, seed)

					// Verify basic properties
					if len(rooms) != tt.yMax {
						t.Errorf("Expected %d rows, got %d", tt.yMax, len(rooms))
					}

					for y := range rooms {
						if len(rooms[y]) != tt.xMax {
							t.Errorf("Expected %d columns in row %d, got %d", tt.xMax, y, len(rooms[y]))
						}
					}

					// Verify start and end points exist
					startX, startY := maze.GetStart()
					endX, endY := maze.GetEnd()

					if rooms[startY][startX] == nil {
						t.Error("Start room is nil")
					}
					if rooms[endY][endX] == nil {
						t.Error("End room is nil")
					}

					if !rooms[startY][startX].IsStart() {
						t.Error("Start room doesn't report as start")
					}
					if !rooms[endY][endX].IsEnd() {
						t.Error("End room doesn't report as end")
					}

					// Verify critical path exists
					criticalPath := maze.GetCriticalPath()
					if len(criticalPath) == 0 {
						t.Error("Critical path is empty")
					}

					// Verify critical path starts at start and ends at end
					if len(criticalPath) > 0 {
						firstRoom := criticalPath[0]
						lastRoom := criticalPath[len(criticalPath)-1]

						fx, fy, _ := firstRoom.GetPosition()
						lx, ly, _ := lastRoom.GetPosition()

						if fx != startX || fy != startY {
							t.Errorf("Critical path doesn't start at start room: expected (%d,%d), got (%d,%d)", startX, startY, fx, fy)
						}
						if lx != endX || ly != endY {
							t.Errorf("Critical path doesn't end at end room: expected (%d,%d), got (%d,%d)", endX, endY, lx, ly)
						}
					}

					// Render the maze beautifully
					rendered := renderMaze(rooms, maze, tt.xMax, tt.yMax)

					// Print the maze (this will show in test output with -v flag)
					t.Logf("\n=== Maze %dx%d (seed: %s) ===\n%s\n", tt.xMax, tt.yMax, seed, rendered)

					// Verify maze has some complexity (not all rooms should exist due to removal)
					roomCount := 0
					for x := 0; x < tt.xMax; x++ {
						for y := 0; y < tt.yMax; y++ {
							if rooms[y][x] != nil {
								roomCount++
							}
						}
					}

					expectedMinRooms := (tt.xMax * tt.yMax) / 2 // At least half should remain
					if roomCount < expectedMinRooms {
						t.Logf("Warning: Maze seems sparse with only %d/%d rooms", roomCount, tt.xMax*tt.yMax)
					}

					// Test connectivity of critical path
					for i := 0; i < len(criticalPath)-1; i++ {
						current := criticalPath[i]
						next := criticalPath[i+1]

						if !current.IsConnectedTo(next) {
							t.Errorf("Critical path broken: room at step %d not connected to step %d", i+1, i+2)
						}
					}

					// Only run first seed for large mazes to avoid too much output
					if tt.xMax >= 16 && i > 0 {
						return
					}
				})
			}
		})
	}
}

// renderMaze creates a beautiful ASCII representation of the maze
func renderMaze(rooms [][]*GridRoom, maze *GridMaze, xMax, yMax int) string {
	var result strings.Builder

	// Create a larger grid to show walls and passages
	displayWidth := xMax*4 + 2
	displayHeight := yMax*2 + 2
	display := make([][]rune, displayHeight)
	for i := range display {
		display[i] = make([]rune, displayWidth)
		for j := range display[i] {
			display[i][j] = ' '
		}
	}

	// Fill in walls initially
	for y := 0; y < displayHeight; y++ {
		for x := 0; x < displayWidth; x++ {
			display[y][x] = '█'
		}
	}

	// Get critical path for highlighting
	criticalPath := maze.GetCriticalPath()
	criticalPathSet := make(map[MazeRoom]bool)
	for _, room := range criticalPath {
		criticalPathSet[room] = true
	}

	roundCount := 0
	// Process each room
	for x := 0; x < xMax; x++ {
		for y := 0; y < yMax; y++ {
			room := rooms[y][x]
			if room == nil {
				continue
			}

			roundCount++

			// Calculate display position
			displayX := x*4 + 2
			displayY := y*2 + 1

			// Choose room symbol
			var roomSymbol rune
			if room.IsStart() {
				roomSymbol = 'S'
			} else if room.IsEnd() {
				roomSymbol = 'E'
			} else if criticalPathSet[room] {
				roomSymbol = '.' // Highlight critical path
			} else if room.IsDeadEnd() {
				roomSymbol = '☠'
			} else {
				roomSymbol = ' '
			}

			// Place room symbol
			display[displayY][displayX] = roomSymbol
			display[displayY][displayX+1] = roomSymbol

			// Draw connections
			connections := room.GetConnections()
			for _, conn := range connections {
				connX, connY, _ := conn.GetPosition()

				// Only draw if connection is to adjacent room to avoid duplicates
				if connX == x && connY == y+1 { // Vert-Down
					display[displayY+1][displayX] = ' '
					display[displayY+1][displayX+1] = ' '
				} else if connX == x+1 && connY == y { // Right
					display[displayY][displayX+2] = ' '
					display[displayY][displayX+3] = ' '
				}
			}
		}
	}

	// Convert display to string with nice borders
	result.WriteString("┌")
	for x := 0; x < displayWidth; x++ {
		result.WriteString("─")
	}
	result.WriteString("┐\n")

	for y := 0; y < displayHeight; y++ {
		result.WriteString("│")
		for x := 0; x < displayWidth; x++ {
			char := display[y][x]
			if char == '█' {
				result.WriteString("█")
			} else {
				result.WriteRune(char)
			}
		}
		result.WriteString("│\n")
	}

	result.WriteString("└")
	for x := 0; x < displayWidth; x++ {
		result.WriteString("─")
	}
	result.WriteString("┘\n")

	// Add legend
	result.WriteString("\nLegend: SS=Start, EE=End, ..=Critical Path, ☠☠=Dead End, ██=Wall\n")

	// Add statistics
	startX, startY := maze.GetStart()
	endX, endY := maze.GetEnd()
	result.WriteString(fmt.Sprintf("Start: (%d,%d), End: (%d,%d), Room Count: %d, Critical Path Length: %d\n",
		startX, startY, endX, endY, roundCount, len(criticalPath)))

	return result.String()
}
