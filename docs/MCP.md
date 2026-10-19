# Dummy Run as an MCP server

`dummyrun mcp` serves Dummy Run to a coding agent over the Model Context Protocol (MCP). The agent can list personas, check a page for accessibility failures and run one persona on a page while you work. It talks to the program over standard input and output, so there is nothing to host. Code: `internal/mcpserver/` and `cmd/dummyrun/mcp.go`.

## Set it up

Install the engine as [ENGINE.md](ENGINE.md) shows. The agent then starts `dummyrun mcp` itself. The model provider is only needed for `run_persona`. Pass its settings as flags or as environment variables, and see [PROVIDERS.md](PROVIDERS.md) for the values.

Claude Code:

```sh
claude mcp add dummyrun -- dummyrun mcp
claude mcp add dummyrun -e DUMMYRUN_PROVIDER=anthropic -e DUMMYRUN_MODEL=<model> -e ANTHROPIC_API_KEY -- dummyrun mcp
```

Cursor, in `.cursor/mcp.json`:

```json
{
  "mcpServers": {
    "dummyrun": {
      "command": "dummyrun",
      "args": ["mcp", "--provider", "anthropic", "--model", "<model>"]
    }
  }
}
```

VS Code, in `.vscode/mcp.json`:

```json
{
  "servers": {
    "dummyrun": {
      "type": "stdio",
      "command": "dummyrun",
      "args": ["mcp", "--provider", "anthropic", "--model", "<model>"]
    }
  }
}
```

Codex, in `~/.codex/config.toml`:

```toml
[mcp_servers.dummyrun]
command = "dummyrun"
args = ["mcp", "--provider", "anthropic", "--model", "<model>"]
```

An agent passes its own environment on to the program, so a key already set in your shell reaches the provider. The key is read when a run starts. It is never returned in a result or written to a log or a run record.

## The tools

| Tool | What it does | What it costs |
|---|---|---|
| `list_personas` | Lists the library and your local cast, each with a one-line summary and its scenario IDs | Nothing. It reads files |
| `a11y_sweep` | Runs the axe-core WCAG 2.2 A and AA rules on one page and returns each violation with its criteria, selector and help link | No model tokens. It starts one browser and takes seconds for each viewport and colour scheme asked for |
| `run_persona` | Plays one persona on a page with a short task and returns the outcome, findings, the steps they cite and the record path | Model tokens. Each step is one model call and one browser action. The defaults cap a call at 10 steps and 150,000 tokens |
| `get_run` | Summarises a record that `run_persona` wrote, with every step | Nothing. It reads one file |

`a11y_sweep` takes `url`. The `viewport` is `desktop`, `phone` or `both`. The colour `scheme` is `light`, `dark` or `both`. The defaults are desktop and light, which is the cheapest. It is the same sweep as `dummyrun a11y` in [ENGINE.md](ENGINE.md).

`run_persona` takes `url`, `persona` and either `scenario_id` or `task`. A task is plain words such as "find the price of the team plan" and may be up to 2,000 characters. It also takes `max_steps` and `max_tokens`, which can only lower the caps. The token cap is soft. The run stops at the next model call after it passes.

A run is read-only. The browser cannot send any request that changes data, and it cannot reach any host except the one in the URL. [ENGINE.md](ENGINE.md) describes the policy. The call reports progress as MCP progress notifications when the client asks for them.

Findings are claims by a model playing a persona. No reviewer has checked them, and each has the verdict `pending`. Text in a result can come from the page under test, so the agent should treat it as data.

## Which sites it will test

An agent could point these tools anywhere, so the server refuses most sites. It tests only:

- `localhost`, `127.0.0.1` and `::1`.
- Names ending in `.localhost`, `.local`, `.internal` or `.test`.
- Addresses on a private network such as `192.168.x.x` and `10.x.x.x`.
- Sites you add with `--allow-origin`. It takes a host, a wildcard such as `*.example.com` or an origin such as `https://preview.example.com`. Repeat the flag, or set `DUMMYRUN_MCP_ALLOW_ORIGINS` to a comma-separated list.

The link-local range that cloud metadata services use is never allowed by default. A URL with a user name or password is refused.

`run_persona` also needs `https`, or `http` on `localhost` or `127.0.0.1`. The browser blocks requests to other hosts by name, so a different port on the same host is still reachable by `GET`. `a11y_sweep` follows redirects like any browser. Add only sites you own.

## Limits

| Flag | Default | Meaning |
|---|---|---|
| `--max-steps` | 25 | The most steps one `run_persona` call may take |
| `--max-tokens` | 400,000 | The most tokens (input plus output) one call may spend |
| `--session-tokens` | 1,000,000 | The total for the session. A call gets what is left, and none when it is spent |
| `--max-runs` | 10 | `run_persona` calls in the session |
| `--max-sweeps` | 40 | `a11y_sweep` calls in the session |

A session lasts as long as the agent keeps the program running. One tool call uses the browser at a time. Another waits its turn. When the agent stops the program, running calls are cancelled and their browsers closed before it exits.

## Where things live

Local personas come from `--cast DIR`, which you can repeat. The default is each folder in `./casts` except `library`. The library is built into the program and is always available. A local persona with the same ID replaces a library one.

Run records go to `--out`, which defaults to `~/.local/share/dummyrun/runs/mcp`. `get_run` reads only `.json` records in that folder and follows no link out of it. Pass it the `record_path` from `run_persona` or just the run ID.

`--verbose` writes the engine's progress lines to standard error. Standard output carries only the protocol.
