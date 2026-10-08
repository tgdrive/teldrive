# Render a nested services.teldrive.settings attrset to TELDRIVE_ environment
# variables. Mirrors internal/config generateEnvMap: path segments are
# uppercased with '-' and '.' converted to '_'.
#
# Only explicitly set (non-null) leaves are emitted; empty maps are skipped.
# Leaf rendering matches the daemon loader: bools as true/false, ints as-is,
# string lists comma-joined, string maps as comma-joined version:key pairs.
{ lib }:
let
  # Dotted settings paths that are leaf maps (generated alongside the options).
  leafMaps = import ./generated-leaf-maps.nix;

  renderScalar = value:
    if builtins.isBool value then
      lib.boolToString value
    else if builtins.isInt value then
      toString value
    else if builtins.isString value then
      value
    else
      throw "teldrive settings: unsupported value ${builtins.toJSON value}";

  renderLeaf = value:
    if builtins.isAttrs value then
      lib.concatStringsSep "," (lib.mapAttrsToList (k: v: "${k}:${renderScalar v}") value)
    else if builtins.isList value then
      lib.concatMapStringsSep "," renderScalar value
    else
      renderScalar value;

  walk = prefix: attrs:
    lib.concatLists (
      lib.mapAttrsToList (
        name: value:
        let
          path = if prefix == "" then name else "${prefix}.${name}";
        in
        if value == null then
          [ ]
        else if builtins.isAttrs value && !(builtins.elem path leafMaps) then
          walk path value
        else
          [
            {
              name = "TELDRIVE_" + lib.toUpper (lib.replaceStrings [ "-" "." ] [ "_" "_" ] path);
              value = renderLeaf value;
            }
          ]
      ) attrs
    );
in
settings: builtins.listToAttrs (walk "" settings)
