{
  description = "DuckDB Sink Processor for nebu";

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
        packages.default = pkgs.buildGoModule {
          pname = "duckdb-sink";
          version = "0.1.0";

          src = ./.;

          # Build from cmd subdirectory
          subPackages = [ "cmd" ];

          # CGO is required for DuckDB
          CGO_ENABLED = 1;

          # Provide build dependencies
          nativeBuildInputs = with pkgs; [
            pkg-config
          ];

          buildInputs = with pkgs; [
            duckdb
          ];

          # DuckDB linkage flags
          CGO_CFLAGS = "-I${pkgs.duckdb.dev}/include";
          CGO_LDFLAGS = "-L${pkgs.duckdb}/lib -lduckdb";

          # Placeholder vendorHash - will need to be updated after first build
          vendorHash = "sha256-AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=";

          meta = with pkgs.lib; {
            description = "DuckDB sink processor for nebu - writes token transfer events to DuckDB";
            homepage = "https://github.com/withObsrvr/nebu";
            license = licenses.mit;
            maintainers = [ ];
          };
        };

        # Development shell with all dependencies
        devShells.default = pkgs.mkShell {
          buildInputs = with pkgs; [
            go
            duckdb
            pkg-config
          ];

          shellHook = ''
            export CGO_ENABLED=1
            export CGO_CFLAGS="-I${pkgs.duckdb.dev}/include"
            export CGO_LDFLAGS="-L${pkgs.duckdb}/lib -lduckdb"
            echo "DuckDB Sink development environment"
            echo "Build with: go build -o duckdb-sink ./cmd"
          '';
        };
      }
    );
}
