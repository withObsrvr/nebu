{
  description = "nebu - Modular Stellar data pipeline framework";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
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
            # Go toolchain
            go
            gopls
            gotools
            go-tools

            # Protobuf tools
            protobuf
            protoc-gen-go
            protoc-gen-go-grpc

            # Build tools
            gnumake

            # Database tools (for testing)
            duckdb

            # Utilities
            jq
            git
          ];

          shellHook = ''
            echo "🚀 nebu development environment"
            echo ""
            echo "Available tools:"
            echo "  go version: $(go version)"
            echo "  protoc version: $(protoc --version)"
            echo "  duckdb version: $(duckdb --version)"
            echo ""
            echo "Quick start:"
            echo "  make build-processors  - Build all processor binaries"
            echo "  make test              - Run tests"
            echo "  make gen-protos        - Generate protobuf code"
            echo "  make test-integration  - Run integration tests"
            echo ""
          '';
        };

        # Package the nebu CLI and processors
        packages.default = pkgs.buildGoModule {
          pname = "nebu";
          version = "0.3.0";
          src = ./.;
          vendorHash = null;

          subPackages = [
            "cmd/nebu"
          ];

          ldflags = [
            "-s"
            "-w"
          ];

          meta = with pkgs.lib; {
            description = "Modular Stellar data pipeline framework";
            homepage = "https://github.com/withObsrvr/nebu";
            license = licenses.asl20;
          };
        };
      }
    );
}
