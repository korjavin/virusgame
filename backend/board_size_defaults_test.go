package main

import "testing"

func TestChallengeBoardDimensions(t *testing.T) {
	tests := []struct {
		name     string
		rows     int
		cols     int
		wantRows int
		wantCols int
	}{
		{name: "omitted defaults both dimensions", wantRows: 12, wantCols: 12},
		{name: "invalid dimensions default independently", rows: 4, cols: 51, wantRows: 12, wantCols: 12},
		{name: "rectangular custom dimensions", rows: 5, cols: 50, wantRows: 5, wantCols: 50},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newHub()
			from := &User{ID: "from", Username: "From"}
			to := &User{ID: "to", Username: "To"}
			h.users[to.ID] = to

			h.handleChallenge(from, &Message{TargetUserID: to.ID, Rows: tt.rows, Cols: tt.cols})

			if len(h.challenges) != 1 {
				t.Fatalf("got %d challenges, want 1", len(h.challenges))
			}
			for _, challenge := range h.challenges {
				if challenge.Rows != tt.wantRows || challenge.Cols != tt.wantCols {
					t.Fatalf("got %dx%d, want %dx%d", challenge.Rows, challenge.Cols, tt.wantRows, tt.wantCols)
				}
			}
		})
	}
}

func TestLobbyBoardDimensions(t *testing.T) {
	tests := []struct {
		name     string
		rows     int
		cols     int
		wantRows int
		wantCols int
	}{
		{name: "omitted defaults both dimensions", wantRows: 12, wantCols: 12},
		{name: "invalid dimensions default independently", rows: -1, cols: 100, wantRows: 12, wantCols: 12},
		{name: "rectangular custom dimensions", rows: 30, cols: 7, wantRows: 30, wantCols: 7},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newHub()
			user := &User{ID: "host", Username: "Host"}

			h.handleCreateLobby(user, &Message{Rows: tt.rows, Cols: tt.cols})

			if len(h.lobbies) != 1 {
				t.Fatalf("got %d lobbies, want 1", len(h.lobbies))
			}
			for _, lobby := range h.lobbies {
				if lobby.Rows != tt.wantRows || lobby.Cols != tt.wantCols {
					t.Fatalf("got %dx%d, want %dx%d", lobby.Rows, lobby.Cols, tt.wantRows, tt.wantCols)
				}
			}
		})
	}
}
