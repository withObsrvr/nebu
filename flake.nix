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
              version = "2.0.0";
              src = prev.fetchFromGitHub {
                owner = "duckdb";
                repo = "duckdb";
                rev = "v${version}";
                hash = "sha256-1fhgPD/QGT/fK+BabX2dbTWOsajS6feNl5+9iw8rvwU=";
              };
              # nixpkgs runs DuckDB's own suite with an explicit list of
              # tests that cannot pass in its sandbox. 2.0.0 adds
              # loadable-extension tests that are not on that list, so the
              # phase fails 25 of 7832 cases — every one under
              # test/extension/, none reachable from how this repo uses
              # duckdb.
              #
              # Maintaining nixpkgs' exclusion list here would mean re-tuning
              # it on every bump. The dev shell needs a working CLI, not a
              # revalidated distribution package.
              #
              # doInstallCheck, not doCheck: the tests run in
              # installCheckPhase, and the same flag feeds the
              # BUILD_UNITTESTS cmake option, so turning it off also stops
              # them being compiled. Setting doCheck here is inert — it does
              # not even change the derivation hash.
              doInstallCheck = false;
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
            just
            gnumake # only for the Makefile->just compatibility shim
            golangci-lint

            # Database tools (for testing)
            duckdb

            # Utilities
            jq
            git
          ];

          shellHook = ''
            # DuckDB 2.0.0 enables "agent mode" when AI_AGENT or CLAUDECODE is
            # set and stdout is not a terminal, which besides reformatting
            # output collapses read_json's inferred schema to one `json`
            # column. Off by default here so a query means the same thing for
            # a person and for an agent.
            export DUCKDB_AGENT_MODE=0

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
            echo "  just                   - List every recipe"
            echo "  just ci                - Everything the CI merge gate enforces"
            echo "  just test              - Run tests"
            echo "  just build-processors  - Build all processor binaries"
            echo "  just gen-protos        - Generate protobuf code"
            echo "  just test-integration  - Run integration tests"
            echo ""
          '';
        };

        # Package the nebu CLI and processors
        packages.default = pkgs.buildGoModule {
          pname = "nebu";
          version = "0.6.15";
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
