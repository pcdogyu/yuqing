package cryptoutil

import "testing"

func TestScoreDirectionAndLabel(t *testing.T) {
	tests := []struct {
		name      string
		text      string
		wantLabel string
		check     func(float64) bool
	}{
		{
			name:      "positive",
			text:      "ETF approval triggers breakout rally",
			wantLabel: "bullish",
			check: func(score float64) bool {
				return score > 0.35
			},
		},
		{
			name:      "negative",
			text:      "黑客攻击导致清算和暴跌",
			wantLabel: "bearish",
			check: func(score float64) bool {
				return score < -0.35
			},
		},
		{
			name:      "empty",
			text:      "   ",
			wantLabel: "neutral",
			check: func(score float64) bool {
				return score == 0
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score := ScoreDirection(tt.text)
			if !tt.check(score) {
				t.Fatalf("ScoreDirection(%q) = %v", tt.text, score)
			}
			if label := DirectionLabel(score); label != tt.wantLabel {
				t.Fatalf("DirectionLabel(%v) = %q, want %q", score, label, tt.wantLabel)
			}
		})
	}
}

func TestDetectReason(t *testing.T) {
	tests := []struct {
		name         string
		text         string
		wantCategory string
		wantLabel    string
	}{
		{
			name:         "regulation",
			text:         "SEC regulation update pressures the market",
			wantCategory: "regulation",
			wantLabel:    "监管政策",
		},
		{
			name:         "fallback",
			text:         "market participants are waiting for the next move",
			wantCategory: "macro",
			wantLabel:    "市场情绪与流动性",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			category, label := DetectReason(tt.text)
			if category != tt.wantCategory || label != tt.wantLabel {
				t.Fatalf("DetectReason(%q) = (%q, %q), want (%q, %q)", tt.text, category, label, tt.wantCategory, tt.wantLabel)
			}
		})
	}
}

func TestClamp01(t *testing.T) {
	tests := []struct {
		input float64
		want  float64
	}{
		{input: -0.2, want: 0},
		{input: 0.4, want: 0.4},
		{input: 1.2, want: 1},
	}

	for _, tt := range tests {
		if got := Clamp01(tt.input); got != tt.want {
			t.Fatalf("Clamp01(%v) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestRound2(t *testing.T) {
	if got := Round2(1.235); got != 1.24 {
		t.Fatalf("Round2(1.235) = %v, want 1.24", got)
	}
}
