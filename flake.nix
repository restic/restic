{
  description = "restic - Backup program that is fast, efficient and secure";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
  };

  outputs = { self, nixpkgs }:
    let
      system = "x86_64-linux";
      pkgs = import nixpkgs { inherit system; };
    in
    {
      packages.${system} = {
        default = pkgs.buildGoModule {
          pname = "restic";
          version = "dev";
          src = self;
          subPackages = [ "cmd/restic" ];
          vendorHash = "sha256-ULyz3M9RDztoVbeuEb5RjIzS51/GN+7/bKR6QBCM7Fg=";
          # Integration tests require permissions not available in the Nix build sandbox
          doCheck = false;
        };
      };

      devShells.${system} = {
        default = pkgs.mkShell {
          buildInputs = [
            pkgs.go
          ];
        };
      };

      checks.${system} = {
        package = self.packages.${system}.default;
        devShell = self.devShells.${system}.default;
      };
    };
}
