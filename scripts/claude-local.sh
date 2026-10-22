#!/usr/bin/env bash
# Claude Code against the fleet's own model: gpt-oss-120b, served by llama.cpp
# on a local server reachable over the VPN
# a few hosts only. Slow, so a
# focused question rather than a long agentic session.
#
# A wrapper rather than `env` in devenv.nix on purpose: ANTHROPIC_AUTH_TOKEN
# replaces the claude.ai login for the session (which also drops the
# connectors), and set shell-wide it would send everyone's every session to
# the local model.
#
# Claude Code's model catalog only knows Anthropic models, so the local one is
# registered through --settings, with behavesAs telling it which capabilities
# to assume; the request still carries the local name. The haiku variable
# covers the background calls (session titles) that would otherwise fail.
set -euo pipefail

url=http://llm.vpn.example.org:8080
model=gpt-oss-120b
settings='{"modelPicker":{"options":[{"model":"gpt-oss-120b","label":"gpt-oss-120b (local)","behavesAs":"claude-opus-4-8"}]}}'

export ANTHROPIC_BASE_URL="$url"
export ANTHROPIC_AUTH_TOKEN=unused
export ANTHROPIC_DEFAULT_HAIKU_MODEL="$model"
exec claude --settings "$settings" --model "$model" "$@"
