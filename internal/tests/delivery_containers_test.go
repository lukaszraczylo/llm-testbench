package tests

import (
	"context"
	"testing"
)

func TestDelDockerLayerCacheBustTest_Eval(t *testing.T) {
	tc := delDockerLayerCacheBustTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{name: "bare object", response: `{"line": 3}`, want: 1},
		{name: "fenced with prose", response: "The culprit is:\n```json\n{\"line\": 3}\n```", want: 1},
		{name: "extra whitespace inside the object", response: `{ "line" : 3 }`, want: 1},
		{name: "wrong: blames the FROM line", response: `{"line": 1}`, want: 0},
		{name: "wrong: blames go build instead", response: `{"line": 5}`, want: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tc.Eval.Evaluate(context.Background(), tt.response)
			if got.Value != tt.want {
				t.Errorf("Eval.Evaluate(%q) = %v, want %v (detail: %s)", tt.response, got.Value, tt.want, got.Detail)
			}
		})
	}
}

func TestDelDockerEntrypointCmdTraceTest_Eval(t *testing.T) {
	tc := delDockerEntrypointCmdTraceTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{name: "exact", response: "python3 app.py --port 8080", want: 1},
		{name: "different case", response: "Python3 App.py --Port 8080", want: 1},
		{name: "surrounding whitespace", response: "  python3 app.py --port 8080\n", want: 1},
		{name: "wrong: only CMD's args", response: "--port 8080", want: 0},
		{name: "wrong: only ENTRYPOINT, drops CMD's default args", response: "python3 app.py", want: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tc.Eval.Evaluate(context.Background(), tt.response)
			if got.Value != tt.want {
				t.Errorf("Eval.Evaluate(%q) = %v, want %v (detail: %s)", tt.response, got.Value, tt.want, got.Detail)
			}
		})
	}
}

func TestDelDockerMultistageSizeBenefitTest_Eval(t *testing.T) {
	tc := delDockerMultistageSizeBenefitTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{
			name:     "correct: multi-stage + scratch",
			response: "Use a multi-stage build: compile in the golang stage, then copy only the binary into a final scratch image with no toolchain at all.",
			want:     1,
		},
		{
			name:     "correct: multi-stage + distroless",
			response: "A multi-stage build lets you copy just the compiled binary into a distroless final image, dropping the entire build toolchain layer.",
			want:     1,
		},
		{
			name:     "correct: multi-stage + alpine",
			response: "With a multi-stage build the final stage can be a minimal alpine base holding only the binary, instead of the full golang image.",
			want:     1,
		},
		{
			name:     "wrong: neither term present",
			response: "Just delete unused files with a RUN rm -rf step at the end of the same image.",
			want:     0,
		},
		{
			name:     "wrong: names the base but never says multi-stage",
			response: "Switch the final image to alpine instead of golang.",
			want:     0.5,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tc.Eval.Evaluate(context.Background(), tt.response)
			if got.Value != tt.want {
				t.Errorf("Eval.Evaluate(%q) = %v, want %v (detail: %s)", tt.response, got.Value, tt.want, got.Detail)
			}
		})
	}
}

func TestDelDockerImageSizeMathTest_Eval(t *testing.T) {
	tc := delDockerImageSizeMathTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{name: "bare number", response: "30", want: 1},
		{name: "sentence form", response: "The final image is 30 MB.", want: 1},
		{name: "with units spelled out", response: "8 + 22 = 30 megabytes.", want: 1},
		{name: "wrong: sums every layer including the discarded build stage", response: "534", want: 0},
		{name: "wrong: only the alpine base, forgets the binary layer", response: "8", want: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tc.Eval.Evaluate(context.Background(), tt.response)
			if got.Value != tt.want {
				t.Errorf("Eval.Evaluate(%q) = %v, want %v (detail: %s)", tt.response, got.Value, tt.want, got.Detail)
			}
		})
	}
}

func TestDelDockerNonrootUserCapsTest_Eval(t *testing.T) {
	tc := delDockerNonrootUserCapsTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{
			name:     "correct: USER + cap-drop=all",
			response: "Add `USER 1000` to the Dockerfile, and run the container with --cap-drop=ALL since it needs no special capabilities.",
			want:     1,
		},
		{
			name:     "correct: USER + drop all capabilities phrasing",
			response: "Create a non-root user with adduser and add a USER instruction; at runtime, drop all capabilities.",
			want:     1,
		},
		{
			name:     "correct: USER + cap-drop all spaced form",
			response: "Set USER app in the image, and pass --cap-drop all at runtime since no privileged syscalls are needed.",
			want:     1,
		},
		{
			name:     "wrong: neither addressed",
			response: "This is fine as-is, since the process only listens on a port.",
			want:     0,
		},
		{
			name:     "wrong: only addresses the user, not capabilities",
			response: "Add a USER instruction pointing at a non-root UID.",
			want:     0.5,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tc.Eval.Evaluate(context.Background(), tt.response)
			if got.Value != tt.want {
				t.Errorf("Eval.Evaluate(%q) = %v, want %v (detail: %s)", tt.response, got.Value, tt.want, got.Detail)
			}
		})
	}
}

func TestDelDockerHealthcheckSemanticsTest_Eval(t *testing.T) {
	tc := delDockerHealthcheckSemanticsTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{name: "bare number", response: "3", want: 1},
		{name: "sentence form", response: "After 3 consecutive failures.", want: 1},
		{name: "labelled form", response: "retries: 3", want: 1},
		{name: "wrong: confuses timeout with retries", response: "3s", want: 0}, // "3s" glues the 3 to a unit letter, so extraction fails and scores 0
		{name: "wrong: confuses interval with retries", response: "30", want: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tc.Eval.Evaluate(context.Background(), tt.response)
			if got.Value != tt.want {
				t.Errorf("Eval.Evaluate(%q) = %v, want %v (detail: %s)", tt.response, got.Value, tt.want, got.Detail)
			}
		})
	}
}

func TestDelDockerDockerignoreEffectTest_Eval(t *testing.T) {
	tc := delDockerDockerignoreEffectTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{name: "exact correct set", response: `["app.js","README.md"]`, want: 1},
		{name: "correct set, different order", response: `["README.md","app.js"]`, want: 1},
		{name: "correct set, fenced", response: "```json\n[\"app.js\",\"README.md\"]\n```", want: 1},
		{
			name:     "wrong: includes an excluded file",
			response: `["app.js","README.md","debug.log"]`,
			want:     2.0 / 3.0,
		},
		{
			name:     "wrong: also excludes a file that should be included",
			response: `["app.js"]`,
			want:     0.5,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tc.Eval.Evaluate(context.Background(), tt.response)
			if got.Value != tt.want {
				t.Errorf("Eval.Evaluate(%q) = %v, want %v (detail: %s)", tt.response, got.Value, tt.want, got.Detail)
			}
		})
	}
}

func TestDelDockerCopyVsAddTest_Eval(t *testing.T) {
	tc := delDockerCopyVsAddTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{name: "all correct", response: `{"scenario_a":"COPY","scenario_b":"ADD"}`, want: 1},
		{name: "all correct fenced", response: "```json\n{\"scenario_a\":\"COPY\",\"scenario_b\":\"ADD\"}\n```", want: 1},
		{name: "all correct lowercase", response: `{"scenario_a":"copy","scenario_b":"add"}`, want: 1},
		{name: "scenario_a wrong", response: `{"scenario_a":"ADD","scenario_b":"ADD"}`, want: 0.5},
		{name: "both swapped", response: `{"scenario_a":"ADD","scenario_b":"COPY"}`, want: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tc.Eval.Evaluate(context.Background(), tt.response)
			if got.Value != tt.want {
				t.Errorf("Eval.Evaluate(%q) = %v, want %v (detail: %s)", tt.response, got.Value, tt.want, got.Detail)
			}
		})
	}
}

func TestDelDockerBuildArgVsEnvTest_Eval(t *testing.T) {
	tc := delDockerBuildArgVsEnvTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{name: "all correct", response: `{"scenario_a":"ARG","scenario_b":"ENV"}`, want: 1},
		{name: "all correct fenced", response: "```json\n{\"scenario_a\":\"ARG\",\"scenario_b\":\"ENV\"}\n```", want: 1},
		{name: "all correct lowercase", response: `{"scenario_a":"arg","scenario_b":"env"}`, want: 1},
		{name: "scenario_b wrong", response: `{"scenario_a":"ARG","scenario_b":"ARG"}`, want: 0.5},
		{name: "both swapped", response: `{"scenario_a":"ENV","scenario_b":"ARG"}`, want: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tc.Eval.Evaluate(context.Background(), tt.response)
			if got.Value != tt.want {
				t.Errorf("Eval.Evaluate(%q) = %v, want %v (detail: %s)", tt.response, got.Value, tt.want, got.Detail)
			}
		})
	}
}

func TestDelDockerLayerCountTest_Eval(t *testing.T) {
	tc := delDockerLayerCountTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{name: "bare number", response: "2", want: 1},
		{name: "sentence form", response: "This Dockerfile adds 2 new layers.", want: 1},
		{name: "explains then answers", response: "Only COPY and RUN create layers here, so the answer is 2.", want: 1},
		{name: "wrong: counts every instruction", response: "8", want: 0},
		{name: "wrong: forgets the RUN layer", response: "1", want: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tc.Eval.Evaluate(context.Background(), tt.response)
			if got.Value != tt.want {
				t.Errorf("Eval.Evaluate(%q) = %v, want %v (detail: %s)", tt.response, got.Value, tt.want, got.Detail)
			}
		})
	}
}
