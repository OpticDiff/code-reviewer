{
  description = "code-reviewer - AI-powered code review CLI";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixpkgs-unstable";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs = { self, nixpkgs, flake-utils }:
    flake-utils.lib.eachDefaultSystem (system:
      let
        pkgs = nixpkgs.legacyPackages.${system};
        version = if (self ? shortRev) then self.shortRev else "dev";
      in
      {
        packages.default = pkgs.buildGoModule {
          pname = "code-reviewer";
          inherit version;
          src = self;
          # Update after go.mod/go.sum changes: ./scripts/update-vendor-hash.sh
          vendorHash = "sha256-FZUO2vyA56DpHazr5eiEApffJe07oHN7iDuoteQmK2Y=";
          subPackages = [ "cmd/code-reviewer" ];
          ldflags = [ "-s" "-w" "-X main.version=${version}" ];
          meta = {
            description = "AI-powered code review CLI";
            homepage = "https://github.com/OpticDiff/code-reviewer";
            mainProgram = "code-reviewer";
          };
        };

        devShells.default = pkgs.mkShell {
          buildInputs = with pkgs; [
            go
            golangci-lint
            git
          ];

          shellHook = ''
            echo "code-reviewer dev shell (Go $(go version | awk '{print $3}' | sed 's/go//'))"
          '';
        };
      });
}
