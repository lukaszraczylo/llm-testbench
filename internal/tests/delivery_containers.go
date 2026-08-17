package tests

import (
	"github.com/lukaszraczylo/llm-testbench/internal/eval"
	"github.com/lukaszraczylo/llm-testbench/internal/testkit"
)

func registerDeliveryContainersTests(r *testkit.Registry) {
	r.Register(delDockerLayerCacheBustTest())
	r.Register(delDockerEntrypointCmdTraceTest())
	r.Register(delDockerMultistageSizeBenefitTest())
	r.Register(delDockerImageSizeMathTest())
	r.Register(delDockerNonrootUserCapsTest())
	r.Register(delDockerHealthcheckSemanticsTest())
	r.Register(delDockerDockerignoreEffectTest())
	r.Register(delDockerCopyVsAddTest())
	r.Register(delDockerBuildArgVsEnvTest())
	r.Register(delDockerLayerCountTest())
}

// delDockerLayerCacheBustDockerfile is the inline numbered Dockerfile for
// delDockerLayerCacheBustTest.
const delDockerLayerCacheBustDockerfile = `1: FROM golang:1.25
2: WORKDIR /app
3: COPY . .
4: RUN go mod download
5: RUN go build -o app .
6: CMD ["./app"]`

// delDockerLayerCacheBustTest: spot the line whose placement busts the
// Docker layer cache on every source change.
//
// ground truth: line 3 (COPY . .) copies the entire source tree, including
// files that change on nearly every commit, before line 4 (go mod
// download). Docker invalidates a layer's cache, and every layer after it,
// as soon as any input to that layer changes - so any single source-file
// edit busts the cache starting at line 3, forcing "go mod download" to
// rerun on every build even when go.mod/go.sum never changed. Copying only
// go.mod/go.sum first, downloading dependencies, and copying the rest of
// the source afterward would let dependency downloads stay cached across
// source-only changes.
func delDockerLayerCacheBustTest() testkit.Test {
	prompt := `Here is a Dockerfile, with line numbers prefixed:

` + delDockerLayerCacheBustDockerfile + `

This Dockerfile reruns "go mod download" on every single build, even when
go.mod and go.sum have not changed. Which line number's placement busts the
Docker layer cache on every source-code change, forcing "go mod download"
to rerun unnecessarily? Respond with only a JSON object: {"line": <line
number as an integer>}`

	return testkit.Test{
		ID:          "docker-layer-cache-bust",
		Category:    "delivery",
		Subcategory: "containers",
		Description: "Spot the COPY . . line placed before dependency download that busts the Docker layer cache on every source change.",
		Prompt:      prompt,
		Eval:        eval.JSONField("line", 3),
	}
}

// delDockerEntrypointCmdTest: trace the exact resulting startup command
// when both ENTRYPOINT and CMD are set in exec form.
//
// ground truth: with both instructions in exec (JSON array) form, CMD's
// array is not run on its own - it supplies the default arguments appended
// after ENTRYPOINT's array, used only when "docker run" is given no
// trailing command. Run with no extra arguments, the process actually
// executed is "python3 app.py --port 8080".
func delDockerEntrypointCmdTraceTest() testkit.Test {
	prompt := `A Dockerfile contains exactly these two instructions:

` + "```dockerfile\n" + `ENTRYPOINT ["python3", "app.py"]
CMD ["--port", "8080"]
` + "```" + `

If the container is run with no extra arguments ("docker run image"), what
is the exact resulting command line executed inside the container? Respond
with only the exact command line, nothing else.`

	return testkit.Test{
		ID:          "docker-entrypoint-cmd-trace",
		Category:    "delivery",
		Subcategory: "containers",
		Description: "Trace ENTRYPOINT+CMD exec-form interaction to the exact resulting startup command line.",
		Prompt:      prompt,
		Eval:        eval.Equals("python3 app.py --port 8080"),
	}
}

// delDockerMultistageSizeBenefitTest: explain how a multi-stage build
// shrinks the final image and name a minimal final-stage base.
func delDockerMultistageSizeBenefitTest() testkit.Test {
	prompt := `A Go service's Dockerfile currently uses a single
"golang:1.25" base image (full Go toolchain, apt package caches, and build
tools) both to compile the binary AND to run it, producing a final image
over 900MB. Explain how switching to a multi-stage build reduces the final
image size, and name the kind of minimal base image you would copy the
compiled binary into for the final stage.`

	evaluator := eval.All(
		eval.W(eval.ContainsAll("multi-stage"), 2),
		eval.W(eval.ContainsAny("scratch", "distroless", "alpine"), 2),
	)

	return testkit.Test{
		ID:          "docker-multistage-size-benefit",
		Category:    "delivery",
		Subcategory: "containers",
		Description: "Explain the multi-stage build size benefit and name a minimal final-stage base (scratch/distroless/alpine).",
		Prompt:      prompt,
		Eval:        evaluator,
	}
}

// delDockerImageSizeMathWant is the total size, in MB, of only the final
// stage's layers in delDockerImageSizeMathTest.
//
// ground truth: only layers in the FINAL stage of a multi-stage build ship
// in the resulting image. The build stage's numbers (72 + 148 + 4 + 310)
// are discarded distractors; the final stage is alpine base (8MB) plus the
// compiled-binary COPY layer (22MB): 8 + 22 = 30.
var delDockerImageSizeMathWant = 8 + 22

// delDockerImageSizeMathTest: sum only the final stage's layer sizes in a
// multi-stage build, ignoring the discarded build-stage layers.
func delDockerImageSizeMathTest() testkit.Test {
	prompt := `A Docker image is built with a multi-stage build:

Build stage (discarded, not part of the final image): base image 72MB, an
apt-get install layer adds 148MB, a COPY of application source adds 4MB,
and a RUN go build layer adds 310MB.

Final stage (this is what actually ships): base image alpine at 8MB, plus a
COPY of the compiled binary from the build stage adds 22MB.

What is the total size, in MB, of the final shipped image? Respond with
only the number.`

	return testkit.Test{
		ID:          "docker-image-size-math",
		Category:    "delivery",
		Subcategory: "containers",
		Description: "Sum only the final stage's layer sizes (8+22=30MB) in a multi-stage build, ignoring discarded build-stage layers.",
		Prompt:      prompt,
		Eval:        eval.Numeric(eval.ExtractLastNumber[int], delDockerImageSizeMathWant, 0),
	}
}

// delDockerNonrootUserCapsTest: require a non-root USER and a fully-dropped
// capability set for a container that needs no privileged syscalls.
//
// ground truth: adding a USER instruction (a numeric non-root UID, or a
// user created via adduser/useradd) stops the container's process from
// running as root. Binding to a port above 1024 needs no special Linux
// capability (only binding below 1024 needs CAP_NET_BIND_SERVICE), so with
// no other privileged operation required, every capability should be
// dropped at runtime (--cap-drop=ALL, with no --cap-add).
func delDockerNonrootUserCapsTest() testkit.Test {
	prompt := `This Dockerfile currently has no USER instruction, so its
process runs as root inside the container. The entrypoint binary only needs
to bind to a port above 1024 - nothing else privileged. Give the Dockerfile
instruction that makes the container run as a non-root user, and name the
Linux capability set it should end up running with at runtime, given it
needs no special privileges.`

	evaluator := eval.All(
		eval.W(eval.ContainsAll("USER"), 2),
		eval.W(eval.ContainsAny("cap-drop=all", "cap-drop all", "drop all capabilities", "capdrop=all", "drop all"), 2),
	)

	return testkit.Test{
		ID:          "docker-nonroot-user-caps",
		Category:    "delivery",
		Subcategory: "containers",
		Description: "Require a USER instruction plus a fully-dropped capability set for a container needing no privileged syscalls.",
		Prompt:      prompt,
		Eval:        evaluator,
	}
}

// delDockerHealthcheckRetriesWant is the number of consecutive failed
// probes before Docker marks the container unhealthy in
// delDockerHealthcheckSemanticsTest, taken directly from the instruction's
// own --retries=3 value.
var delDockerHealthcheckRetriesWant = 3

// delDockerHealthcheckSemanticsTest: read the HEALTHCHECK retries value
// correctly rather than the interval or timeout values.
func delDockerHealthcheckSemanticsTest() testkit.Test {
	prompt := `Given this Dockerfile instruction:

` + "```dockerfile\n" + `HEALTHCHECK --interval=30s --timeout=3s --retries=3 CMD curl -f http://localhost:8080/health || exit 1
` + "```" + `

Assuming every probe run so far has failed, after how many consecutive
FAILED probes does Docker mark the container's health status as
"unhealthy"? Respond with only the integer.`

	return testkit.Test{
		ID:          "docker-healthcheck-semantics",
		Category:    "delivery",
		Subcategory: "containers",
		Description: "Read a HEALTHCHECK instruction's --retries value as the consecutive-failure count before unhealthy status.",
		Prompt:      prompt,
		Eval:        eval.Numeric(eval.ExtractLastNumber[int], delDockerHealthcheckRetriesWant, 0),
	}
}

// delDockerignoreFile is the inline .dockerignore content for
// delDockerDockerignoreEffectTest. Deliberately uses no negation patterns
// (unlike delGitignoreFile's git-specific negation trap): Docker's own
// negation-after-directory-exclusion behavior differs between the classic
// builder and BuildKit and is not as uniformly documented as git's, so
// relying on it here would not be a deterministic, single-answer test.
const delDockerignoreFile = `node_modules
*.log
.git`

// delDockerDockerignoreEffectWant is the sorted set of files from
// delDockerDockerignoreEffectTest's inline file list that are NOT excluded
// by delDockerignoreFile and so are sent to the Docker build context.
//
// ground truth: "node_modules" excludes node_modules/pkg/index.js. "*.log"
// excludes debug.log. ".git" excludes .git/config. app.js and README.md
// match no pattern and are included.
var delDockerDockerignoreEffectWant = []string{"README.md", "app.js"}

// delDockerDockerignoreEffectTest: identify which files are NOT excluded by
// a .dockerignore and so are sent to the Docker daemon's build context.
func delDockerDockerignoreEffectTest() testkit.Test {
	prompt := `Here is a repository's .dockerignore:

` + "```\n" + delDockerignoreFile + "\n```" + `

And here is the full list of files that exist in the build directory:
app.js, node_modules/pkg/index.js, debug.log, README.md, .git/config

Which of these files WILL be sent to the Docker daemon as part of the build
context (i.e. are NOT excluded by .dockerignore)? Respond with only a JSON
array of the included file paths.`

	return testkit.Test{
		ID:          "docker-dockerignore-effect",
		Category:    "delivery",
		Subcategory: "containers",
		Description: "Identify which files a .dockerignore leaves in the build context that gets sent to the Docker daemon.",
		Prompt:      prompt,
		Eval:        eval.JSONStringSet(delDockerDockerignoreEffectWant),
	}
}

// delDockerCopyVsAddTest: pick COPY for plain local files, ADD for the one
// feature COPY lacks - local tar auto-extraction.
//
// ground truth: COPY is Docker's own recommended default - transparent,
// predictable, does only a file copy. ADD additionally auto-extracts a
// local tar/gzip archive into the destination directory as part of the same
// instruction (and can also fetch remote URLs, which Docker's own best
// practices discourage) - the tar-extraction behavior is the one thing ADD
// does that COPY cannot, making it the correct choice specifically for
// scenario_b.
func delDockerCopyVsAddTest() testkit.Test {
	prompt := `For each of these two situations, should you use "COPY" or
"ADD" in the Dockerfile?

scenario_a: "Copy a local directory of static assets into the image, with
no extraction or URL-fetching behavior needed - just the files as they are
on disk."
scenario_b: "Add a local .tar.gz archive to the image and have it
automatically extracted into the destination directory as part of the same
instruction."

Respond with only a JSON object:
{"scenario_a":"COPY"|"ADD","scenario_b":"COPY"|"ADD"}`

	evaluator := eval.Mean(
		eval.JSONField("scenario_a", "COPY"),
		eval.JSONField("scenario_b", "ADD"),
	)

	return testkit.Test{
		ID:          "docker-copy-vs-add",
		Category:    "delivery",
		Subcategory: "containers",
		Description: "Choose COPY for a plain file copy and ADD for its unique local-tar-auto-extraction behavior.",
		Prompt:      prompt,
		Eval:        evaluator,
	}
}

// delDockerBuildArgVsEnvTest: pick ARG for a build-time-only value and ENV
// for a value the running container must expose.
//
// ground truth: ARG values exist only during the build and are not present
// in the final image's runtime environment or in "docker inspect"'s Env
// list, matching scenario_a's requirement. ENV values are baked into the
// image's runtime config and are what "docker run -e" overrides and what a
// running process reads via getenv, matching scenario_b.
func delDockerBuildArgVsEnvTest() testkit.Test {
	prompt := `For each of these two situations, should the value be set
with "ARG" or "ENV" in the Dockerfile?

scenario_a: "A value is needed only at image-build time (for example,
selecting which internal build number to embed into a version label), and
must NOT be present in the final running container's environment or in
'docker inspect' output."
scenario_b: "A value must be configurable at container start time (for
example by an operator running 'docker run -e ...'), without rebuilding the
image, and must be readable by the running process."

Respond with only a JSON object:
{"scenario_a":"ARG"|"ENV","scenario_b":"ARG"|"ENV"}`

	evaluator := eval.Mean(
		eval.JSONField("scenario_a", "ARG"),
		eval.JSONField("scenario_b", "ENV"),
	)

	return testkit.Test{
		ID:          "docker-build-arg-vs-env",
		Category:    "delivery",
		Subcategory: "containers",
		Description: "Choose ARG for a build-time-only value and ENV for a value the running container must expose.",
		Prompt:      prompt,
		Eval:        evaluator,
	}
}

// delDockerLayerCountDockerfile is the inline numbered Dockerfile for
// delDockerLayerCountTest.
const delDockerLayerCountDockerfile = `1: FROM alpine:3.20
2: WORKDIR /app
3: COPY app /app/app
4: RUN chmod +x /app/app
5: ENV PORT=8080
6: EXPOSE 8080
7: USER 1000
8: ENTRYPOINT ["/app/app"]`

// delDockerLayerCountWant is the number of new filesystem layers this
// Dockerfile adds on top of its base image.
//
// ground truth: only RUN, COPY, and ADD instructions create a new
// filesystem layer; WORKDIR, ENV, EXPOSE, USER, ENTRYPOINT, CMD, and LABEL
// only update image metadata/config and add no layer. This Dockerfile has
// exactly one COPY (line 3) and one RUN (line 4): 2 new layers.
var delDockerLayerCountWant = 2

// delDockerLayerCountTest: count only the new filesystem layers a
// Dockerfile adds on top of its base image, distinguishing layer-creating
// instructions (RUN/COPY/ADD) from metadata-only ones.
func delDockerLayerCountTest() testkit.Test {
	prompt := `Here is a Dockerfile, with line numbers prefixed:

` + delDockerLayerCountDockerfile + `

Not counting whatever layers already exist inside the alpine:3.20 base
image itself, how many NEW filesystem layers does building this Dockerfile
add on top of that base? Respond with only the number.`

	return testkit.Test{
		ID:          "docker-layer-count",
		Category:    "delivery",
		Subcategory: "containers",
		Description: "Count only the layer-creating instructions (RUN/COPY/ADD) in a Dockerfile, excluding metadata-only ones.",
		Prompt:      prompt,
		Eval:        eval.Numeric(eval.ExtractLastNumber[int], delDockerLayerCountWant, 0),
	}
}
