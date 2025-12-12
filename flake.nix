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

        # Go tools for protobuf generation
        goTools = with pkgs; [
          (buildGoModule {
            pname = "protoc-gen-go";
            version = "1.31.0";
            src = fetchFromGitHub {
              owner = "protocolbuffers";
              repo = "protobuf-go";
              rev = "v1.31.0";
              sha256 = "sha256-7QQB7RxYVrCVP+tgYCCTqG4cP+dP3F5Hy5GEr/q9m+w=";
            };
            vendorHash = "sha256-y23FFhtgbqN0VdLYBtqhyf2s0Xqz4+1aHNK+kPOC2OQ=";
            subPackages = [ "cmd/protoc-gen-go" ];
          })
          (buildGoModule {
            pname = "protoc-gen-go-grpc";
            version = "1.3.0";
            src = fetchFromGitHub {
              owner = "grpc";
              repo = "grpc-go";
              rev = "cmd/protoc-gen-go-grpc/v1.3.0";
              sha256 = "sha256-1R1WfqoReUSPHP/jLkjJG9LRA2LoT7KVNGOJBevRNog=";
            };
            vendorHash = "sha256-dq7aWUIebv/DWmLCCJqM7gwJVA7FOhd1g3+A/sZGT5A=";
            subPackages = [ "cmd/protoc-gen-go-grpc" ];
          })
        ];

      in
      {
        devShells.default = pkgs.mkShell {
          buildInputs = with pkgs; [
            # Go toolchain
            go_1_21
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
            echo "  make build       - Build all binaries"
            echo "  make test        - Run tests"
            echo "  make gen-protos  - Generate protobuf code"
            echo ""

            # Add Go bin to PATH for installed tools
            export PATH="$HOME/go/bin:$PATH"

            # Ensure protoc plugins are available
            if ! command -v protoc-gen-go &> /dev/null; then
              echo "Installing protoc-gen-go..."
              go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
            fi

            if ! command -v protoc-gen-go-grpc &> /dev/null; then
              echo "Installing protoc-gen-go-grpc..."
              go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
            fi
          '';
        };

        # Package the nebu CLI
        packages.default = pkgs.buildGoModule {
          pname = "nebu";
          version = "0.3.0";
          src = ./.;
          vendorHash = null; # Will be computed on first build

          # Build all processors
          subPackages = [
            "cmd/nebu"
            "examples/processors/token-transfer/cmd"
            "examples/processors/usdc-filter/cmd"
            "examples/processors/amount-filter/cmd"
            "examples/processors/time-window/cmd"
            "examples/processors/dedup/cmd"
            "examples/processors/json-file-sink/cmd"
          ];

          ldflags = [
            "-s"
            "-w"
            "-X main.version=0.3.0"
          ];

          meta = with pkgs.lib; {
            description = "Modular Stellar data pipeline framework";
            homepage = "https://github.com/withObsrvr/nebu";
            license = licenses.asl20;
            maintainers = [];
          };
        };
      }
    );
}
