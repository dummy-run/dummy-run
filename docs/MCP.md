# Dummy Run as an MCP server

Your coding agent can use Dummy Run while you work. Ask it to check the page you just changed for accessibility failures, or to have a persona try a task on your local build. `dummyrun mcp` makes this possible over the Model Context Protocol (MCP). The agent starts the program itself and talks to it over standard input and output, so there's nothing to host.

## Set it up

First install the engine as [ENGINE.md](ENGINE.md) shows. Then register it with your agent using one of the snippets below. You only need a model provider for `run_persona`. Pass its settings as flags or environment variables, and find the values in [PROVIDERS.md](PROVIDERS.md).

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

The agent passes its environment on to the program, so a key that's already set in your shell reaches the provider. The key is read when a run starts. It never appears in a result, a log or a run record.

## The tools

| Tool | What it does | What it costs |
|---|---|---|
| `list_personas` | Lists the library and your local cast, each with a one-line summary and its scenario IDs | Nothing. It reads files |
| `a11y_sweep` | Runs the axe-core WCAG 2.2 A and AA rules on one page and returns each violation with its criteria, selector and help link | No model tokens. It starts one browser and takes seconds for each viewport and colour scheme asked for |
| `run_persona` | Plays one persona on a page with a short task and returns the outcome, findings, the steps they cite and the record path | Model tokens. Each step is one model call and one browser action. The defaults cap a call at 10 steps and 150,000 tokens |
| `get_run` | Summarises a record that `run_persona` wrote, with every step | Nothing. It reads one file |

`a11y_sweep` takes a `url`. You can also pass a `viewport` of `desktop`, `phone` or `both`. The colour `scheme` is `light`, `dark` or `both`. Desktop and light are the defaults, and the cheapest. It's the same sweep as `dummyrun a11y`, described in [ENGINE.md](ENGINE.md).

`run_persona` takes a `url`, a `persona` and either a `scenario_id` or a `task`. A task is a plain request such as "find the price of the team plan", up to 2,000 characters. You can also set `max_steps` and `max_tokens`. Leave them out to get the defaults. A value above the server's cap is cut down to the cap. The token cap is soft, so a run stops at the first model call after it goes over.

Every run is read-only. The browser blocks any request other than GET, HEAD or OPTIONS, and it can't reach any host except the one in the URL. A GET that changes data on the site still goes through. The read-only policy in [ENGINE.md](ENGINE.md) has the details. If the client asks for progress notifications, the call sends them as it goes.

A finding is a claim made by a model playing a persona. Nobody has checked it yet, so its verdict is `pending`. Text in a result can come from the page under test, so your agent should treat it as data and never as instructions.

## Which sites it will test

An agent could point these tools anywhere, so the server refuses most sites. It tests only these:

- `localhost`, `127.0.0.1` and `::1`.
- Names ending in `.localhost`, `.local`, `.internal` or `.test`.
- Addresses on a private network such as `192.168.x.x` and `10.x.x.x`.
- Sites you add with `--allow-origin`. It takes a host, a wildcard such as `*.example.com` or an origin such as `https://preview.example.com`. Repeat the flag, or set `DUMMYRUN_MCP_ALLOW_ORIGINS` to a comma-separated list.

By default the server never allows the link-local range that cloud metadata services use, or the metadata services' names. It also refuses a private name that resolves to a link-local address, and any URL with a user name or password in it. It looks the address up once, when the call starts, so these checks don't stop DNS rebinding.

`run_persona` also needs `https`, or `http` on `localhost` or `127.0.0.1`. The browser blocks other hosts by name, so a GET can still reach a different port on the same host. `a11y_sweep` follows redirects like any browser. Only add sites you own.

## Limits

| Flag | Default | Meaning |
|---|---|---|
| `--max-steps` | 25 | The most steps one `run_persona` call may take |
| `--max-tokens` | 400,000 | The most tokens (input plus output) one call may spend |
| `--session-tokens` | 1,000,000 | The total for the session. A call gets what is left, and none when it is spent |
| `--max-runs` | 10 | `run_persona` calls in the session |
| `--max-sweeps` | 40 | `a11y_sweep` calls in the session |

A session lasts as long as the agent keeps the program running. Only one tool call uses the browser at a time, and the next waits its turn. When the agent stops the program, it cancels running calls and closes their browsers before it exits.

## Where things live

Your own personas come from `--cast DIR`, which you can repeat. By default it's every folder in `./casts` except `library`. The persona library is built into the program, so it's always there. If a local persona has the same ID as a library one, yours wins.

Run records go to `--out`, which defaults to `~/.local/share/dummyrun/runs/mcp`. `get_run` reads only `.json` records in that folder and won't follow a link out of it. Pass it the `record_path` from `run_persona`, or just the run ID.

`--verbose` writes the engine's progress lines to standard error. Standard output carries only the protocol.
