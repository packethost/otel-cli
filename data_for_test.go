package main_test

// Data structures and data for functional testing of otel-cli.

// TODO: strip defaults from the data structures (might mean dumping them again or a bit of manual work...)
// TODO: rename Fixture.Filename to Fixture.Name or something like that
// TODO: add instructions for adding more tests

import "github.com/equinix-labs/otel-cli/cmd"

type FixtureConfig struct {
	CliArgs []string `json:"cli_args"`
	Env     map[string]string
	// timeout for how long to wait for the whole test in failure cases
	TestTimeoutMs int `json:"test_timeout_ms"`
	// when true this test will be excluded under go -test.short mode
	// TODO: maybe move this up to the suite?
	IsLongTest bool `json:"is_long_test"`
	// for timeout tests we need to start the server to generate the endpoint
	// but do not want it to answer when otel-cli calls, this does that
	StopServerBeforeExec bool `json:"stop_server_before_exec"`
	// run this fixture in the background, starting its server and otel-cli
	// instance, then let those block in the background and continue running
	// serial tests until it's "foreground" by a second fixtue with the same
	// description in the same file
	Background bool `json:"background"`
	Foreground bool `json:"foreground"`
}

// mostly mirrors cmd.StatusOutput but we need more
type Results struct {
	// same as cmd.StatusOutput but copied because embedding doesn't work for this
	Config      cmd.Config        `json:"config"`
	SpanData    map[string]string `json:"span_data"`
	Env         map[string]string `json:"env"`
	Diagnostics cmd.Diagnostics   `json:"diagnostics"`
	// these are specific to tests...
	CliOutput     string `json:"output"`         // merged stdout and stderr
	Spans         int    `json:"spans"`          // number of spans received
	Events        int    `json:"events"`         // number of events received
	TimedOut      bool   `json:"timed_out"`      // true when test timed out
	CommandFailed bool   `json:"command_failed"` // otel-cli failed / was killed
}

// Fixture represents a test fixture for otel-cli.
type Fixture struct {
	Description string        `json:"description"`
	Filename    string        `json:"-"` // populated at runtime
	Config      FixtureConfig `json:"config"`
	Expect      Results       `json:"expect"`
}

// FixtureSuite is a list of Fixtures that run serially.
type FixtureSuite []Fixture

var suites = []FixtureSuite{
	// otel-cli should not do anything when it is not explicitly configured"
	{
		{
			Filename: "nothing configured",
			Config: FixtureConfig{
				CliArgs: []string{"status"},
			},
			Expect: Results{
				Config: cmd.DefaultConfig(),
				Diagnostics: cmd.Diagnostics{
					IsRecording: false,
					NumArgs:     1,
					OtelError:   "",
				},
			},
		},
	},
	// setting minimum envvars should result in a span being received
	{
		{
			Filename: "minimum configuration (recording)",
			Config: FixtureConfig{
				CliArgs:       []string{"status"},
				Env:           map[string]string{"OTEL_EXPORTER_OTLP_ENDPOINT": "{{endpoint}}"},
				TestTimeoutMs: 1000,
			},
			Expect: Results{
				// otel-cli should NOT set insecure when it auto-detects localhost
				Config: cmd.DefaultConfig().
					WithEndpoint("{{endpoint}}").
					WithInsecure(false),
				SpanData: map[string]string{
					"span_id":  "*",
					"trace_id": "*",
				},
				Env: map[string]string{
					"OTEL_EXPORTER_OTLP_ENDPOINT": "{{endpoint}}",
				},
				Diagnostics: cmd.Diagnostics{
					IsRecording:       true,
					NumArgs:           1,
					DetectedLocalhost: true,
					ParsedTimeoutMs:   1000,
					OtelError:         "",
				},
				Spans: 1,
			},
		},
	},
	// otel is configured but there is no server listening so it should time out silently
	{
		{
			Filename: "timeout with no server",
			Config: FixtureConfig{
				CliArgs: []string{"span", "--timeout", "1s"},
				Env: map[string]string{
					"OTEL_EXPORTER_OTLP_ENDPOINT": "{{endpoint}}",
				},
				// this needs to be less than the timeout in CliArgs
				TestTimeoutMs:        500,
				IsLongTest:           true, // can be skipped with `go test -short`
				StopServerBeforeExec: true, // there will be no server listening
			},
			Expect: Results{
				Config: cmd.DefaultConfig(),
				// we want and expect a timeout and failure
				TimedOut:      true,
				CommandFailed: true,
			},
		},
	},
	// otel-cli span with no OTLP config should do and print nothing
	{
		{
			Filename: "otel-cli span (unconfigured, non-recording)",
			Config: FixtureConfig{
				CliArgs: []string{"span", "--service", "main_test.go", "--name", "test-span-123", "--kind", "server"},
			},
			Expect: Results{Config: cmd.DefaultConfig()},
		},
	},
	// otel-cli with minimal config span sends a span that looks right
	{
		{
			Filename: "otel-cli span (recording)",
			Config: FixtureConfig{
				CliArgs: []string{"span", "--service", "main_test.go", "--name", "test-span-123", "--kind", "server"},
				Env: map[string]string{
					"OTEL_EXPORTER_OTLP_ENDPOINT": "{{endpoint}}",
				},
				TestTimeoutMs: 1000,
			},
			Expect: Results{
				Config: cmd.DefaultConfig(),
				SpanData: map[string]string{
					"is_sampled": "true",
					"span_id":    "*",
					"trace_id":   "*",
				},
				Spans: 1,
			},
		},
	},
	// otel-cli span --print-tp actually prints
	{
		{
			Filename: "otel-cli span --print-tp",
			Config: FixtureConfig{
				CliArgs: []string{"span", "--tp-print"},
				Env:     map[string]string{"TRACEPARENT": "00-f6c109f48195b451c4def6ab32f47b61-a5d2a35f2483004e-01"},
			},
			Expect: Results{
				Config: cmd.DefaultConfig(),
				CliOutput: "" + // empty so the text below can indent and line up
					"# trace id: 00000000000000000000000000000000\n" +
					"#  span id: 0000000000000000\n" +
					"TRACEPARENT=00-f6c109f48195b451c4def6ab32f47b61-a5d2a35f2483004e-01\n",
			},
		},
	},
	// otel-cli span --print-tp propagates traceparent even when not recording
	{
		{
			Filename: "otel-cli span --tp-print --tp-export (non-recording)",
			Config: FixtureConfig{
				CliArgs: []string{"span", "--tp-print", "--tp-export"},
				Env: map[string]string{
					"TRACEPARENT": "00-f6c109f48195b451c4def6ab32f47b61-a5d2a35f2483004e-01",
				},
			},
			Expect: Results{
				Config: cmd.DefaultConfig(),
				CliOutput: "" +
					"# trace id: 00000000000000000000000000000000\n" +
					"#  span id: 0000000000000000\n" +
					"export TRACEPARENT=00-f6c109f48195b451c4def6ab32f47b61-a5d2a35f2483004e-01\n",
			},
		},
	},
	// otel-cli span background, non-recording, this uses the suite functionality
	// and background tasks, which are a little clunky but get the job done
	{
		{
			Filename: "otel-cli span background (nonrecording)",
			Config: FixtureConfig{
				CliArgs:       []string{"span", "background", "--timeout", "1s", "--sockdir", "."},
				TestTimeoutMs: 2000,
				Background:    true,  // sorta like & in shell
				Foreground:    false, // must be true later, like `fg` in shell
			},
			Expect: Results{Config: cmd.DefaultConfig()},
		},
		{
			Filename: "otel-cli span event",
			Config: FixtureConfig{
				CliArgs: []string{"span", "event", "--name", "an event happened", "--attrs", "ima=now,mondai=problem", "--sockdir", "."},
			},
			Expect: Results{Config: cmd.DefaultConfig()},
		},
		{
			Filename: "otel-cli span end",
			Config: FixtureConfig{
				CliArgs: []string{"span", "end", "--sockdir", "."},
			},
			Expect: Results{Config: cmd.DefaultConfig()},
		},
		{
			Filename: "fg span background",
			Config: FixtureConfig{
				Foreground: true, // bring it back (fg) and finish up
			},
			Expect: Results{Config: cmd.DefaultConfig()},
		},
	},
	// otel-cli span background, in recording mode
	{
		{
			Filename: "otel-cli span background (recording)",
			Config: FixtureConfig{
				CliArgs:       []string{"span", "background", "--timeout", "1s", "--sockdir", "."},
				Env:           map[string]string{"OTEL_EXPORTER_OTLP_ENDPOINT": "{{endpoint}}"},
				TestTimeoutMs: 2000,
				Background:    true,
				Foreground:    false,
			},
			Expect: Results{
				Config: cmd.DefaultConfig(),
				SpanData: map[string]string{
					"span_id":  "*",
					"trace_id": "*",
				},
				Spans:  1,
				Events: 1,
			},
		},
		{
			Description: "otel-cli span event",
			Filename:    "81-span-background.json",
			Config: FixtureConfig{
				CliArgs: []string{"span", "event", "--name", "an event happened", "--attrs", "ima=now,mondai=problem", "--sockdir", "."},
			},
			Expect: Results{Config: cmd.DefaultConfig()},
		},
		{
			Filename: "otel-cli span end",
			Config: FixtureConfig{
				CliArgs: []string{"span", "end", "--sockdir", "."},
			},
			Expect: Results{Config: cmd.DefaultConfig()},
		},
		{
			Filename: "foreground otel-cli and finish the test",
			Config: FixtureConfig{
				Foreground: true, // fg
			},
			Expect: Results{Config: cmd.DefaultConfig()},
		},
	},
	// otel-cli exec runs echo
	{
		{
			Filename: "otel-cli span exec echo",
			Config: FixtureConfig{
				CliArgs: []string{"exec", "--service", "main_test.go", "--name", "test-span-123", "--kind", "server", "echo hello world"},
				Env: map[string]string{
					"OTEL_EXPORTER_OTLP_ENDPOINT": "{{endpoint}}",
					"TRACEPARENT":                 "00-edededededededededededededed9000-edededededededed-01",
				},
			},
			Expect: Results{
				Config: cmd.DefaultConfig(),
				SpanData: map[string]string{
					"is_sampled": "true",
					"span_id":    "*",
					"trace_id":   "edededededededededededededed9000",
				},
				CliOutput: "hello world\n",
				Spans:     1,
			},
		},
	},
	// otel-cli exec runs otel-cli exec
	{
		{
			Filename: "otel-cli span exec (nested)",
			Config: FixtureConfig{
				CliArgs: []string{"exec", "--service", "main_test.go", "--name", "test-span-123", "--kind", "server", "./otel-cli", "exec", "--tp-ignore-env", "echo hello world $TRACEPARENT"},
				Env: map[string]string{
					"OTEL_EXPORTER_OTLP_ENDPOINT": "{{endpoint}}",
				},
			},
			Expect: Results{
				Config:    cmd.DefaultConfig(),
				SpanData:  map[string]string{"is_sampled": "true", "span_id": "*", "trace_id": "*"},
				CliOutput: "hello world\n",
				Spans:     2,
			},
		},
	},
}
