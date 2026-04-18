package llm

// Ollama exposes an OpenAI-compatible API at http://localhost:11434/v1.
// The factory wires it through OpenAICompatClient — this file exists so the
// project layout described in BUILD.md is complete and future Ollama-specific
// tweaks (model listing, streaming) have a natural home.

const OllamaDefaultBaseURL = "http://localhost:11434/v1"
