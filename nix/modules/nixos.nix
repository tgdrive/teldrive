{ config, lib, pkgs, ... }:
let
  cfg = config.services.teldrive;
  toEnv = import ./to-env.nix { inherit lib; };
in
{
  options.services.teldrive = {
    enable = lib.mkEnableOption "Teldrive Telegram-backed cloud storage server";

    package = lib.mkOption {
      type = lib.types.package;
      default = pkgs.teldrive;
      defaultText = lib.literalExpression "pkgs.teldrive";
      description = "Teldrive package to run (requires the teldrive flake overlay).";
    };

    settings = lib.mkOption {
      type = lib.types.submodule {
        options = import ./generated-options.nix { inherit lib; };
      };
      default = { };
      description = "Teldrive settings, injected as TELDRIVE_ environment variables. Only explicitly set values are passed; the daemon applies its own defaults otherwise. See `just nix-generate` and internal/config for the full schema.";
    };

    environmentFile = lib.mkOption {
      type = lib.types.nullOr lib.types.path;
      default = null;
      example = "/run/secrets/teldrive.env";
      description = "File with TELDRIVE_ assignments (e.g. via sops-nix) for secrets such as database.url and security keys. Takes precedence over settings.";
    };

    user = lib.mkOption {
      type = lib.types.str;
      default = "teldrive";
      description = "User account to provision. The service does not set User=; override systemd.services.teldrive.serviceConfig.User to run as this account.";
    };

    group = lib.mkOption {
      type = lib.types.str;
      default = "teldrive";
      description = "Group account to provision. The service does not set Group=; override systemd.services.teldrive.serviceConfig.Group to run as this group.";
    };
  };

  config = lib.mkIf cfg.enable {
    users.users = lib.mkIf (cfg.user == "teldrive") {
      teldrive = {
        isSystemUser = true;
        group = cfg.group;
        description = "Teldrive service";
      };
    };

    users.groups = lib.mkIf (cfg.group == "teldrive") {
      teldrive = { };
    };

    systemd.services.teldrive = {
      description = "Teldrive Telegram-backed cloud storage server";
      wantedBy = [ "multi-user.target" ];
      after = [ "network.target" ];
      environment = toEnv cfg.settings;
      serviceConfig =
        {
          ExecStart = "${cfg.package}/bin/teldrive run";
          StateDirectory = "teldrive";
          WorkingDirectory = "/var/lib/teldrive";
          Restart = "on-failure";
        }
        // lib.optionalAttrs (cfg.environmentFile != null) {
          EnvironmentFile = cfg.environmentFile;
        };
    };
  };
}
