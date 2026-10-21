# Model providers

Dummy Run plays a persona with a model you choose. Pick the backend with `--provider`. Any model you use
needs tool calling and must accept images.

| `--provider` | Backend | Sign-in |
|---|---|---|
| `azure` | Azure OpenAI | `az login` or another Azure default credential |
| `openai-compatible` | Any service or local server with the OpenAI chat API | An API key from the variable named by `--api-key-env`, or none for a local server |
| `anthropic` | Anthropic Messages API | `ANTHROPIC_API_KEY`, or the variable named by `--api-key-env` |
| `gemini` | Gemini Developer API | `GEMINI_API_KEY`, or the variable named by `--api-key-env` |
| `vertex` | Gemini on Vertex AI | Application Default Credentials, with `--project` and `--region` |
| `bedrock` | Amazon Bedrock Converse | The AWS default credential chain |

The engine reads your API key from the environment when the run starts. It never prints the key or writes
it to a run record. The record keeps only the provider kind, the model and the region.

## Leave the provider out

For a first run you don't have to choose. `dummyrun try` and `dummyrun run` look at your environment when you
leave out `--provider`, take the first match in the table and print what they chose on the first line:

```text
Using Anthropic claude-haiku-5-5 (ANTHROPIC_API_KEY is set)
```

Name `--model`, `--region` or `--endpoint` yourself and your value wins over the default. If nothing matches,
the command lists the ways to set one up. Azure comes first because `DUMMYRUN_AZURE_ENDPOINT` is a variable only
Dummy Run reads, so setting it is a deliberate choice.

This table is generated from the code, so it matches this version of the engine. Each default is a low-priced model that the vendor's own page
lists with tool calling and image input, as of this version. Models change, so check the vendor page before you
rely on one, and pass `--model` to use another.

<!-- docgen:defaults begin -->

| Order | Chosen when | Provider | Model | Why this model |
|---|---|---|---|---|
| 1 | `DUMMYRUN_AZURE_ENDPOINT` is set | `azure` | yours: pass `--model` | Deployment names are your own, so there is no default model. Pass `--model` and `--region`. |
| 2 | `OPENAI_API_KEY` is set | `openai-compatible` with `--reasoning-effort none` | `gpt-6-luna` | The cheapest GPT-6 model, with function calling and image input. On chat completions it calls tools only with reasoning off, so the default sends `--reasoning-effort none`. [OpenAI models](https://developers.openai.com/api/docs/models/gpt-6-luna) |
| 3 | `ANTHROPIC_API_KEY` is set | `anthropic` | `claude-haiku-5-5` | The fastest and cheapest Claude, with tool use and image input. [Anthropic models](https://platform.claude.com/docs/en/about-claude/models/overview) |
| 4 | `OLLAMA_HOST` is set, or Ollama answers on `localhost:11434` | `openai-compatible` | `gemma4:e4b` | A local model that lists both tools and vision, so your data stays on your machine. Run `ollama pull gemma4:e4b` once. [Ollama gemma4](https://ollama.com/library/gemma4) |

<!-- docgen:defaults end -->

With `--provider anthropic` and no `--model`, you get the Anthropic default. Every other provider needs `--model`.
Ollama must have the model: `ollama pull gemma4:e4b` is one download of a few gigabytes, and the first run
checks for it. Ollama also needs room for a screenshot in every request. Its default context of 4096 tokens fills
up after a dozen steps, so start it with a larger one: `OLLAMA_CONTEXT_LENGTH=16384 ollama serve`. Expect a small local model to be slower and less careful than a hosted one. On a busy laptop with no GPU, a single
step can take a minute, and a call that takes longer than two minutes ends the run. For a first run, a hosted key is the quicker way in.

## What each backend can do

Backends differ in a few ways that change how a run behaves. If a backend can't force a tool call, the
last step asks the model to call `finish`, and asks again if it answers in prose. If a backend takes images
inside tool results, the screenshot goes there. Otherwise it follows as a separate user message.

The table below is generated from the code, so it matches this version of the engine.

<!-- docgen:providers begin -->

| Kind | Forced tool | Images in tool results | Reasoning effort | Responses API | Cached tokens reported | Seed sent | Region |
|---|---|---|---|---|---|---|---|
| `anthropic` | no | yes | no | no | yes | no | `global` unless declared |
| `azure-foundry` | yes | no | yes | yes | yes | yes | you declare it |
| `bedrock` | no | no | no | no | yes | no | `--region` decides the endpoint, `eu-west-2` by default |
| `gemini-api` | yes | no | no | no | yes | yes | `global` unless declared |
| `openai-compatible` | no | no | yes | yes | yes | yes | you declare it |
| `vertex-gemini` | yes | no | no | no | yes | yes | `--region` decides the endpoint |

Every kind supports tool calling, images in user messages and structured output.

<!-- docgen:providers end -->

## OpenAI-compatible services

To use a hosted service that speaks the OpenAI API, point `--endpoint` at its base URL and name the
variable that holds your key:

```sh
export SCW_SECRET_KEY=...        # the key stays in your environment
dummyrun run ... --provider openai-compatible \
  --endpoint https://api.scaleway.ai/v1 --api-key-env SCW_SECRET_KEY \
  --model <model> --region fr-par
```

The run record needs a region, so pass the one you know the service runs in, or `local` for a server on
your machine. The engine sends `tool_choice: auto` and `max_tokens`, and adds `max_completion_tokens` only
for `api.openai.com`. It ignores any `OPENAI_API_KEY`, `OPENAI_ORG_ID` or `OPENAI_PROJECT_ID` in your
environment, so a key for one vendor never reaches another.

Here's what each vendor's own docs say. Where a page didn't say, the cell tells you. Models and regions
change, so check the vendor before you rely on a detail.

| Service | Base URL | Tools and images | Data region | Vendor docs |
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

Scaleway's and OVHcloud's pages didn't show a base URL, so confirm both with the vendor before your
first run.

### Local servers

With a local server, your data stays on your machine.

| Server | Base URL | Notes | Vendor docs |
|---|---|---|---|
| Ollama | `http://localhost:11434/v1` | Raise the context past its 4096-token default, such as `OLLAMA_CONTEXT_LENGTH=16384 ollama serve`. Tools work. Images work as base64 only. The page lists `tool_choice` as unsupported, so the engine uses the finish instruction. Pick a model that lists `tools` and `vision` | [Ollama OpenAI compatibility](https://docs.ollama.com/api/openai-compatibility) |
| vLLM | `http://localhost:8000/v1` | Start it with `--enable-auto-tool-choice` and a `--tool-call-parser` for the model | [vLLM tool calling](https://docs.vllm.ai/en/latest/features/tool_calling/) |
| llama.cpp server | The host and `--port` you start it with, plus `/v1` | Tool calls need the `--jinja` flag. Images need a multimodal projector and are marked experimental | [llama.cpp server](https://github.com/ggml-org/llama.cpp/blob/master/tools/server/README.md) |

## Reasoning effort and the Responses API

Some OpenAI models take tool calls over chat completions only when reasoning is off. Others need the
Responses API for tools at all. OpenAI's function calling guide says GPT-6 Astra and GPT-6.1 Sol need it.
Two flags cover both cases, for `openai-compatible` and `azure`:

- `--reasoning-effort none|minimal|low|medium|high|xhigh` sets the effort. Leave it out to use the model's default.
- `--openai-api responses` switches from chat completions to the Responses API.

On the Responses API the engine sends the whole conversation with each call and sets `store` to false, so
the service doesn't store the responses for later use. That isn't the same as zero data retention, which each vendor
controls separately. Unless the effort is `none`, it asks for encrypted reasoning and passes it back
alongside the tool call it belongs to, as OpenAI asks for reasoning models.

Check the model's page for the setting it needs. If a model wants reasoning off for chat tool calls, use
`--reasoning-effort none`. On Azure, `--api-version` sets the API version for both APIs.

## Sampling seed

`--seed N` sends a sampling seed to Azure OpenAI and OpenAI-compatible services over chat completions, and
to Gemini on both the Developer API and Vertex AI. The Responses API, Anthropic and Bedrock don't take one.
On those, the run record has `seed: null` and the log says the seed wasn't sent. A seed makes sampling
repeatable only as far as the vendor promises. Gemini accepts seeds up to 2147483647, and the engine clamps
larger ones.

## Cached input tokens

The usage block in a run record can include `cached_input_tokens`, the part of `input_tokens` that the
service answered from its prompt cache. Compare the two to see what caching saves you. The field is left
out when a service doesn't report it.

## Anthropic

The engine uses the official Go SDK. It always sends `tool_choice: auto` with one tool at a time, because
forcing a tool returns a 400 error on current models. So the last step asks the model to call `finish`.
Screenshots go inside the tool result. See [Anthropic tool use](https://platform.claude.com/docs/en/agents-and-tools/tool-use/overview).
You can't pick a region for this API, so the record says `global` unless you pass `--region`.

## Gemini

`gemini` signs in with an API key and uses the Developer API. `vertex` uses Application Default Credentials
from `gcloud auth application-default login`, a service account or workload identity. It also needs
`--project` and `--region`, such as `europe-west2`, and calls go to that region. Gemini 3 models return thought signatures
with function calls, and the engine keeps them and sends them back. See
[Gemini function calling](https://ai.google.dev/gemini-api/docs/function-calling).

If `GOOGLE_API_KEY` or `GEMINI_API_KEY` is set in your shell, the SDK may sign in with it instead. Unset
both when you use `vertex`.

## Amazon Bedrock

The engine uses the Converse API and the AWS default credential chain (environment, shared config, SSO or
a role). `--region` defaults to `eu-west-2` (London). `--model` takes a model ID or an inference profile ID.
Some profiles route across regions, so check a profile's regions if residency matters to you. Bedrock
doesn't force a tool call, so the last step asks the model to call `finish`. See the
[Converse API](https://docs.aws.amazon.com/bedrock/latest/userguide/conversation-inference.html).

The Bedrock backend has been tested against recorded replies but not yet against a live AWS account. If
something doesn't work, please open an issue.

## Azure OpenAI

[ENGINE.md](ENGINE.md) shows an Azure run from start to finish. Azure also takes `--reasoning-effort` and
`--openai-api responses`.
