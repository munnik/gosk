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
        pkgs:
        pkgs.buildGoModule {
          pname = "gosk";
          version = self.shortRev or self.dirtyShortRev or "dev";

          src = self;

          vendorHash = "sha256-r//mle76pC1hTy3CGVczsjoBup2NaiU+HeJaIVKov7Y=";

          doCheck = false; # tests require a database available

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
      overlays.default = final: _prev: { gosk = goskFor final; };

      packages = eachSystem (
        system:
        let
          pkgs = nixpkgs.legacyPackages.${system};
        in
        {
          gosk = goskFor pkgs;
          default = goskFor pkgs;
        }
      );

      formatter = eachSystem (system: nixpkgs.legacyPackages.${system}.nixfmt);
    };
}
