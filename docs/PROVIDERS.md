# Model providers

The engine runs a persona on a model you choose. `--provider` picks the backend. Each backend needs
tool calling and images. Code: `internal/provider/`.

| `--provider` | Backend | Sign-in |
|---|---|---|
| `azure` | Azure OpenAI | `az login` or another Azure default credential |
| `openai-compatible` | Any service or local server with the OpenAI chat API | An API key from the variable named by `--api-key-env`, or none for a local server |
| `anthropic` | Anthropic Messages API | `ANTHROPIC_API_KEY`, or the variable named by `--api-key-env` |
| `gemini` | Gemini Developer API | `GEMINI_API_KEY`, or the variable named by `--api-key-env` |
| `vertex` | Gemini on Vertex AI | Application Default Credentials, with `--project` and `--region` |
| `bedrock` | Amazon Bedrock Converse | The AWS default credential chain |

An API key is read from the environment when the run starts. It is never printed or written to a run
record. The record stores the provider kind, the model and the region only.

## What each backend can do

The runner reads a capability table (`internal/provider/capabilities.go`) and never branches on a
provider name. Two entries matter at run time. Where a backend cannot force a tool, the last step of a run
sends an instruction to call `finish` and asks again if the model answers in prose. Where a backend takes
images in tool results, the screenshot travels inside the tool result. Elsewhere it follows as a user message.
The table is written from the code by `go generate ./cmd/dummyrun`, so edit the code and not the table.

<!-- docgen:providers begin -->

| Kind | Forced tool | Images in tool results | Reasoning effort | Responses API | Cached tokens reported | Region |
|---|---|---|---|---|---|---|
| `anthropic` | no | yes | no | no | yes | `global` unless declared |
| `azure-foundry` | yes | no | yes | yes | yes | you declare it |
| `bedrock` | no | no | no | no | yes | `--region` decides the endpoint, `eu-west-2` by default |
| `gemini-api` | yes | no | no | no | yes | `global` unless declared |
| `openai-compatible` | no | no | yes | yes | yes | you declare it |
| `vertex-gemini` | yes | no | no | no | yes | `--region` decides the endpoint |

Every kind supports tool calling, images in user messages and structured output.

<!-- docgen:providers end -->

## OpenAI-compatible services

```sh
export SCW_SECRET_KEY=...        # the key stays in your environment
dummyrun run ... --provider openai-compatible \
  --endpoint https://api.scaleway.ai/v1 --api-key-env SCW_SECRET_KEY \
  --model <model> --region fr-par
```

`--endpoint` is the base URL. `--region` is where you know the service runs, as the record needs a value.
Use `local` for a server on your machine. The engine sends `tool_choice: auto` and `max_tokens`.
It sends `max_completion_tokens` only to `api.openai.com`. Ambient `OPENAI_API_KEY`, `OPENAI_ORG_ID` and
`OPENAI_PROJECT_ID` values are ignored, so a key for one vendor never reaches another.

The notes below come from each vendor's own page on the date the engine was written. Where a page did not
say, the cell says so. Check the vendor before you rely on a detail, as models and regions change.

| Service | Base URL | Tools and images | Data region | Source |
|---|---|---|---|---|
| OpenAI | `https://api.openai.com/v1` | Tools yes. Images are in the vision guide, not checked here | Not checked here | [OpenAI function calling](https://developers.openai.com/api/docs/guides/function-calling) |
| OpenRouter | `https://openrouter.ai/api/v1` | Tools and base64 images in user messages. Support varies by model | Not stated on the page | [OpenRouter API](https://openrouter.ai/docs/api-reference/overview) |
| Mistral La Plateforme | `https://api.mistral.ai/v1` | Tools yes. Images not stated on the page | Not stated on the page | [Mistral API](https://docs.mistral.ai/api/) |
| Scaleway Generative APIs | `https://api.scaleway.ai/v1`, not confirmed on the page | Not confirmed from the page | Check the vendor's data privacy page | [Scaleway docs](https://www.scaleway.com/en/docs/generative-apis/reference-content/openai-compatibility/) |
| Nebius Token Factory | `https://api.tokenfactory.nebius.com/v1/` | Not stated on the page | Not stated on the page | [Nebius docs](https://docs.tokenfactory.nebius.com/) |
| OVHcloud AI Endpoints | On each model's page | Vision models exist. Function calling has its own guide | Not stated on the page | [OVHcloud guide](https://docs.ovhcloud.com/en/guides/public-cloud/ai-machine-learning/ai-endpoints-getting-started) |
| Together | `https://api.together.ai/v1` | Tools yes. Base64 images yes. `detail` is ignored | Not stated on the page | [Together compatibility](https://docs.together.ai/docs/openai-api-compatibility) |
| Fireworks | `https://api.fireworks.ai/inference/v1` | Tools yes. `auto`, `none`, `required` and a named function | Not stated on the page | [Fireworks compatibility](https://docs.fireworks.ai/tools-sdks/openai-compatibility) and [function calling](https://docs.fireworks.ai/guides/function-calling) |
| DeepInfra | `https://api.deepinfra.com/v1/openai` | Lists `tools` and `tool_choice`. Vision has its own page | Not stated on the page | [DeepInfra chat](https://docs.deepinfra.com/chat/overview) |
| Groq | `https://api.groq.com/openai/v1` | Tool use yes on most models. Chat images not covered on the page | Not stated on the page | [Groq OpenAI compatibility](https://console.groq.com/docs/openai) and [tool use](https://console.groq.com/docs/tool-use) |

The Scaleway and OVHcloud rows are the least certain, because their pages did not show the base URL when
fetched. Confirm both on the vendor page before a first run, and send a pull request with what you find.

### Local servers

| Server | Base URL | Notes | Source |
|---|---|---|---|
| Ollama | `http://localhost:11434/v1` | Tools work. Images work as base64 only. The page lists `tool_choice` as unsupported, so the engine uses the finish instruction. Pick a model that lists `tools` and `vision` | [Ollama OpenAI compatibility](https://docs.ollama.com/api/openai-compatibility) |
| vLLM | `http://localhost:8000/v1` | Start it with `--enable-auto-tool-choice` and a `--tool-call-parser` for the model | [vLLM tool calling](https://docs.vllm.ai/en/latest/features/tool_calling/) |
| llama.cpp server | The host and `--port` you start it with, plus `/v1` | Tool calls need the `--jinja` flag. Images need a multimodal projector and are marked experimental | [llama.cpp server](https://github.com/ggml-org/llama.cpp/blob/master/tools/server/README.md) |

Data stays on your machine with a local server.

## Reasoning effort and the Responses API

Some OpenAI models only take tool calls over chat completions when reasoning is off. Others need the
Responses API for tools. OpenAI's function calling guide says GPT-6 Astra and GPT-6.1 Sol require it.
Two flags cover both, for `openai-compatible` and `azure`:

- `--reasoning-effort none|minimal|low|medium|high|xhigh` sets the effort. Leave it out to use the model's default.
- `--openai-api responses` uses the Responses API instead of chat completions.

With the Responses API the engine sends the whole conversation each call and sets `store` to false, so the
service keeps nothing. Unless the effort is `none`, it asks for encrypted reasoning and passes it back next to
the tool call it belongs to, as OpenAI's guide asks for reasoning models.

Check the model's own page for the setting it needs. A model that wants `none` for chat tool calls is a
case for `--reasoning-effort none`. Azure uses the same API version setting as chat (`--api-version`).

## Cached input tokens

Usage in the run record has an optional `cached_input_tokens`. It is the part of `input_tokens` the service
answered from its prompt cache. Compare it with `input_tokens` to see what caching saves. It is left out when a
service does not report it.

## Anthropic

Uses the official Go SDK. `tool_choice` is always `auto`, with one tool at a time, because forced tool use
returns 400 on current models. The last step sends an instruction to call `finish`. Screenshots go inside the
tool result. Source: [Anthropic tool use](https://platform.claude.com/docs/en/agents-and-tools/tool-use/overview).
The API has no region choice in the engine, so the record says `global` unless you pass `--region`.

## Gemini

`gemini` uses an API key and the Developer API. `vertex` uses Application Default Credentials (`gcloud auth
application-default login`, a service account or workload identity) with `--project` and `--region`, such as
`europe-west2`. Calls go to that region. Gemini 3 models return thought signatures with function calls. The engine
keeps them and sends them back. Source: [Gemini function calling](https://ai.google.dev/gemini-api/docs/function-calling).
An ambient `GOOGLE_API_KEY` or `GEMINI_API_KEY` can change how the SDK signs in, so unset them for `vertex`.

## Amazon Bedrock

Uses the Converse API and the AWS default credential chain (environment, shared config, SSO or a role).
`--region` defaults to `eu-west-2` (London). `--model` is a model ID or an inference profile ID. Some profiles
route across regions, so check the profile's regions if residency matters. The engine does not force a tool on
Bedrock and sends the finish instruction instead.
Source: [Converse API](https://docs.aws.amazon.com/bedrock/latest/userguide/conversation-inference.html).
This backend was tested against recorded replies only, as no AWS account was used.

## Azure OpenAI

See `docs/ENGINE.md`. Azure takes `--reasoning-effort` and `--openai-api responses` as well.

## Tests

`go test ./internal/provider/` runs unit tests for every provider and one contract test that each must
pass. Replies are fixtures in `internal/provider/testdata/`, served by a local test server, so CI needs no key.
The OpenAI-compatible chat fixtures were recorded from a local Ollama. The others follow each vendor's
documented format. `FuzzDecodeResponses` feeds random bytes to every decoder.

To try a local model: `DUMMYRUN_OLLAMA_MODEL=gemma4:e4b go test -run TestOllamaLive -v ./internal/provider/`.
The test is skipped when the variable is unset.
