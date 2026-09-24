{
  description = "Go SignalK implementation and more";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-26.05";
    systems.url = "github:nix-systems/default-linux";
  };

  outputs =
    {
      self,
      nixpkgs,
      systems,
    }:
    let
      eachSystem = nixpkgs.lib.genAttrs (import systems);

      goskFor =
        {
          pkgs,
          runTests ? false,
        }:
        pkgs.buildGoModule {
          pname = "gosk";
          version = self.shortRev or self.dirtyShortRev or "dev";

          src = self;

          vendorHash = "sha256-r//mle76pC1hTy3CGVczsjoBup2NaiU+HeJaIVKov7Y=";

          # Off for the package that gets installed, on for the flake
          # check. database/'s suite starts its own PostgreSQL and needs the
          # timescaledb extension, which is unfree in nixpkgs - see the
          # checks output below, which allows unfree for that build alone so
          # `nix build` of the binary stays free of it.
          doCheck = runTests;

          nativeCheckInputs = pkgs.lib.optionals runTests [
            (pkgs.postgresql.withPackages (p: [ p.timescaledb ]))
          ];

          ldflags = [
            "-s"
            "-w"
            "-X github.com/munnik/gosk/version.Version=${self.shortRev or self.dirtyShortRev or "dev"}"
            "-X github.com/munnik/gosk/version.Commit=${self.rev or self.dirtyRev or "unknown"}"
          ];

          env.CGO_ENABLED = 0;

          meta = with pkgs.lib; {
            description = "Go SignalK implementation and more";
            homepage = "https://github.com/munnik/gosk";
            license = licenses.asl20;
            platforms = platforms.linux;
            mainProgram = "gosk";
          };
        };
    in
    {
      overlays.default = final: _prev: { gosk = goskFor { pkgs = final; }; };

      packages = eachSystem (
        system:
        let
          pkgs = nixpkgs.legacyPackages.${system};
        in
        {
          gosk = goskFor { inherit pkgs; };
          default = goskFor { inherit pkgs; };
        }
      );

      # `nix flake check` builds this, which runs `go test ./...` - including
      # database/, which initdb's a throwaway PostgreSQL in the build
      # sandbox. nixpkgs is instantiated here rather than taken from
      # legacyPackages because timescaledb is unfree, and this keeps that
      # confined to the tests instead of pushing NIXPKGS_ALLOW_UNFREE onto
      # anyone building the binary.
      checks = eachSystem (
        system:
        let
          pkgs = import nixpkgs {
            inherit system;
            config.allowUnfree = true;
          };
        in
        {
          tests = goskFor {
            inherit pkgs;
            runTests = true;
          };
        }
      );

      formatter = eachSystem (system: nixpkgs.legacyPackages.${system}.nixfmt);
    };
}
