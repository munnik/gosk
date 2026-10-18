#!/usr/bin/env bash
# codex against the fleet's own model: gpt-oss-120b, served by llama.cpp on
# the-office-server (hosts/the-office-server/services/llm in the nix repo) and reachable over the VPN from
# the laptop and server blocks only. llama-server speaks the Responses API
# codex uses, so this is only a provider definition; no API key.
#
# A wrapper rather than a provider in ~/.codex/config.toml so that plain
# `codex` keeps going to OpenAI and nobody's config needs editing.
set -euo pipefail

url=http://llm.vpn.example.org:8080/v1

exec codex \
  -c model_provider=sumo \
  -c 'model_providers.sumo.name="the-office-server"' \
  -c "model_providers.sumo.base_url=\"$url\"" \
  -c 'model_providers.sumo.wire_api="responses"' \
  -c 'model="gpt-oss-120b"' \
  "$@"
