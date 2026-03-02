{
  pkgs,
  inputs,
  lib,
  ...
}:
let
  pkgs-unstable = import inputs.nixpkgs-unstable { inherit (pkgs.stdenv) system; };
  zellij-layout-kld = ".config/zellij/layout.kdl";
in
{
  packages = with pkgs; [
    gdlv
    gnugrep
    gofumpt
    golangci-lint
    golangci-lint-langserver
    goreleaser
    pre-commit
    prettier
    prettierd
    rr
    shellcheck
    shfmt
    taplo
    vscode-json-languageserver
  ];

  overlays = [ (_: _: { inherit (pkgs-unstable) delve; }) ];

  languages.go = {
    enable = true;
    enableHardeningWorkaround = true;
    package = pkgs-unstable.go;
  };

  files."${zellij-layout-kld}".text = ''
    layout {
      tab name="Helix" {
        pane size=1 borderless=true {
          plugin location="tab-bar"
        }
        pane split_direction="vertical" {
          pane command="hx"
          pane
        }
        pane size=1 borderless=true {
          plugin location="status-bar"
        }
      }
      tab name="Lazygit" {
        pane size=1 borderless=true {
          plugin location="tab-bar"
        }
        pane command="lazygit"
        pane size=1 borderless=true {
          plugin location="status-bar"
        }
      }
      tab name="Vessel" {
        pane size=1 borderless=true {
          plugin location="tab-bar"
        }
        pane
        pane size=1 borderless=true {
          plugin location="status-bar"
        }
      }
    }
  '';
  enterShell = ''
    if [ -z $ZELLIJ ]; then
      ${lib.getExe pkgs.zellij} --layout ${zellij-layout-kld}
    fi
  '';

  git-hooks.hooks = {
    # markdown
    mdsh.enable = true;

    # nix
    nixfmt.enable = true;
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
