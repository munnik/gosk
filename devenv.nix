{ pkgs, ... }: {
  packages = with pkgs; [ gnugrep prettierd shellcheck shfmt taplo ];

  languages.go = {
    enable = true;
    enableHardeningWorkaround = true;
  };

  git-hooks.hooks = {
    # markdown
    mdsh.enable = true;

    # nix
    nixfmt-classic.enable = true;
    deadnix.enable = true;
    # flake-checker.enable = true;
    nil.enable = true;
    statix.enable = true;

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
