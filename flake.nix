{
  description = "nebu - Modular Stellar data pipeline framework";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs = { self, nixpkgs, flake-utils }:
    flake-utils.lib.eachDefaultSystem (system:
      let
        pkgs = import nixpkgs {
          inherit system;
          config.packageOverrides = prev: {
            duckdb = prev.duckdb.overrideAttrs (oldAttrs: rec {
              version = "1.5.1";
              src = prev.fetchFromGitHub {
                owner = "duckdb";
                repo = "duckdb";
                rev = "v${version}";
                hash = "sha256-FygBpfhvezvUbI969Dta+vZOPt6BnSW2d5gO4I4oB2A=";
              };
            });
          };
        };
      in
      {
        devShells.default = pkgs.mkShell {
          buildInputs = with pkgs; [
            flyctl
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
            # Set custom prompt
            export PS1='\n\[\033[1;35m\][nebu]\[\033[0m\] \[\033[1;32m\]\w\[\033[0m\] \$ '

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
          version = "0.5.0";
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
