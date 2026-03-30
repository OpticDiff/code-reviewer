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
      in
      {
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
