{ pkgs, inputs, ... }:
let
  # Keep AI coding tools current independently of devenv's main package set.
  unstable = import inputs.nixpkgs-unstable {
    inherit (pkgs) config;
    system = pkgs.stdenv.hostPlatform.system;
  };
in
{
  claude.code.enable = true;

  packages = with pkgs; [
    unstable.claude-code
    unstable.codex
    goreleaser
    prettierd
    shellcheck
    shfmt
    taplo
  ];

  scripts = {
    claude-local = {
      exec = ''exec "$DEVENV_ROOT/scripts/claude-local.sh" "$@"'';
      description = "Claude Code against gpt-oss-120b on the-office-server (VPN only) instead of Anthropic.";
    };
    codex-local = {
      exec = ''exec "$DEVENV_ROOT/scripts/codex-local.sh" "$@"'';
      description = "codex against gpt-oss-120b on the-office-server (VPN only) instead of OpenAI.";
    };
  };

  languages = {
    go = {
      enable = true;
      enableHardeningWorkaround = true;
    };
    nix.enable = true;
  };

  git-hooks.hooks = {
    # markdown
    mdsh.enable = true;

    # nix
    nixfmt.enable = true;
    deadnix.enable = true;

    # yaml
    yamllint.enable = true;

    # git
    check-merge-conflicts.enable = true;

    # various
    check-added-large-files.enable = true;
    check-case-conflicts.enable = true;
    check-executables-have-shebangs.enable = true;
    check-shebang-scripts-are-executable.enable = true;
    detect-private-keys.enable = true;
    end-of-file-fixer.enable = true;
    fix-byte-order-marker.enable = true;
    mixed-line-endings.enable = true;
    treefmt.enable = true;
    trim-trailing-whitespace.enable = true;
    trufflehog.enable = true;
  };
}
