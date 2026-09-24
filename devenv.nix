{ pkgs, ... }:
{
  claude.code.enable = true;

  packages = with pkgs; [
    claude-code
    golangci-lint
    goreleaser
    prettierd
    shellcheck
    shfmt
    taplo
  ];

  languages = {
    go = {
      enable = true;
      enableHardeningWorkaround = true;
    };
    nix.enable = true;
  };

  git-hooks.hooks = {
    # go
    # Only forbidigo is on, see .golangci.yml - this is here to enforce
    # the version 7 UUID rule, not to start linting gosk broadly.
    golangci-lint.enable = true;

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
