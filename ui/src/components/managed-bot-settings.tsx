import { Button, Input, Label, TextField } from "@heroui/react";
import { useState } from "react";
import { $api } from "@/api/client";
import { ApiError, userMessage } from "@/api/errors";
import { SettingsSection } from "@/components/settings-layout";
import { newIdempotencyKey } from "@/features/shared/idempotency";
import { t } from "@/lib/localization";
import { getQueryClient } from "@/lib/queryClient";

function botError(error: unknown): string {
  if (error instanceof ApiError) {
    const messages: Record<string, string> = {
      bot_manager_permission:
        "Activa Bot Management Mode para este bot en BotFather y comprueba su token.",
      bot_username_occupied:
        "El usuario ya está ocupado. Puedes recuperar un bot tuyo si utiliza este mismo gestor.",
      bot_username_invalid:
        "Usa entre 5 y 32 letras, números o guiones bajos y termina el usuario en bot.",
      bot_create_limit: "Tu cuenta de Telegram alcanzó el límite de bots que puede crear.",
      bot_flood_wait: "Telegram solicita una pausa. Espera antes de volver a intentarlo.",
      managed_bots_unavailable: "Crear bots requiere una conexión real con Telegram.",
      managed_bot_telegram_error:
        "Telegram no pudo completar la solicitud. Reintenta con el mismo usuario para recuperar una creación interrumpida.",
    };
    if (messages[error.code]) return t(messages[error.code]);
  }
  return userMessage(error);
}

export function ManagedBotSettings({
  bots,
  onCreated,
}: {
  bots: { id: number; username?: string }[];
  onCreated: () => Promise<void>;
}) {
  const [manager, setManager] = useState("");
  const [verified, setVerified] = useState(false);
  const [verifying, setVerifying] = useState(false);
  const [name, setName] = useState("");
  const [username, setUsername] = useState("");
  const [available, setAvailable] = useState(false);
  const [message, setMessage] = useState("");
  const [error, setError] = useState("");
  const check = $api.useMutation("post", "/v1/bots/managed/check-username");
  const create = $api.useMutation("post", "/v1/bots/managed");
  const busy = verifying || check.isPending || create.isPending;
  const validUsername = /^[a-z0-9_]{2,29}bot$/i.test(username);

  const verify = async () => {
    setVerifying(true);
    setError("");
    setMessage("");
    setVerified(false);
    try {
      const result = await getQueryClient().fetchQuery({
        ...$api.queryOptions("get", "/v1/bots/{botId}/manager", {
          params: { path: { botId: Number(manager) } },
        }),
        staleTime: 0,
      });
      setVerified(result.canManageBots);
      setMessage(t("Gestor verificado: @{0}", [result.username]));
    } catch (error) {
      setError(botError(error));
    } finally {
      setVerifying(false);
    }
  };

  const checkUsername = async () => {
    setError("");
    setMessage("");
    setAvailable(false);
    try {
      const result = await check.mutateAsync({ body: { username } });
      setAvailable(result.available);
      setMessage(t("El usuario está disponible."));
    } catch (error) {
      setError(botError(error));
    }
  };

  const createBot = async () => {
    setError("");
    setMessage("");
    try {
      const result = await create.mutateAsync({
        params: { header: { "Idempotency-Key": newIdempotencyKey() } },
        body: { managerBotId: Number(manager), name: name.trim(), username },
      });
      setMessage(
        result.recovered
          ? t("Bot recuperado y añadido: @{0}", [username])
          : t("Bot creado y añadido: @{0}", [username]),
      );
      setName("");
      setUsername("");
      setAvailable(false);
      await onCreated();
    } catch (error) {
      setError(botError(error));
    }
  };

  return (
    <SettingsSection
      title={t("Crear bots desde Teldrive")}
      description={t(
        "Usa tu sesión de Telegram y un bot gestor para crear bots sin copiar sus tokens.",
      )}
    >
      <div className="grid gap-4 p-5">
        <p className="text-sm text-muted">
          {t(
            "Importa abajo el token de tu bot gestor. Activa una sola vez Bot Management Mode en sus ajustes de BotFather y luego selecciónalo aquí.",
          )}{" "}
          <a
            href="https://t.me/BotFather"
            target="_blank"
            rel="noreferrer"
            className="text-accent underline"
          >
            BotFather
          </a>
        </p>
        <div className="flex flex-col gap-2 sm:flex-row sm:items-end">
          <div className="grid min-w-0 flex-1 gap-1">
            <label htmlFor="managed-bot-manager" className="text-sm">
              {t("Bot gestor")}
            </label>
            <select
              id="managed-bot-manager"
              value={manager}
              disabled={busy}
              className="min-h-11 w-full rounded-xl border border-border bg-field px-3 text-foreground"
              onChange={(event) => {
                setManager(event.target.value);
                setVerified(false);
                setError("");
                setMessage("");
              }}
            >
              <option value="">{t("Selecciona un bot importado")}</option>
              {bots.map((bot) => (
                <option key={bot.id} value={bot.id}>
                  {bot.username ? `@${bot.username}` : bot.id}
                </option>
              ))}
            </select>
          </div>
          <Button onPress={() => void verify()} isDisabled={!manager || busy} isPending={verifying}>
            {t("Verificar gestor")}
          </Button>
        </div>
        <TextField value={name} onChange={setName} isDisabled={busy}>
          <Label>{t("Nombre del nuevo bot")}</Label>
          <Input maxLength={64} placeholder={t("Mi bot de almacenamiento")} />
        </TextField>
        <TextField
          value={username}
          onChange={(value) => {
            setUsername(value.trim().replace(/^@/, ""));
            setAvailable(false);
            setError("");
            setMessage("");
          }}
          isDisabled={busy}
        >
          <Label>{t("Usuario del nuevo bot")}</Label>
          <Input
            maxLength={32}
            placeholder="mi_almacenamiento_bot"
            autoCapitalize="none"
            autoCorrect="off"
            spellCheck={false}
          />
        </TextField>
        <p className="text-xs text-muted">
          {t(
            "El usuario debe terminar en bot. Si una creación se interrumpe, usa el mismo usuario y gestor para recuperarla.",
          )}
        </p>
        <div className="flex flex-wrap gap-2">
          <Button
            variant="secondary"
            onPress={() => void checkUsername()}
            isDisabled={!validUsername || busy}
            isPending={check.isPending}
          >
            {t("Comprobar usuario")}
          </Button>
          <Button
            onPress={() => void createBot()}
            isDisabled={
              !verified ||
              !bots.some((bot) => String(bot.id) === manager) ||
              !name.trim() ||
              !validUsername ||
              busy
            }
            isPending={create.isPending}
          >
            {t("Crear y añadir bot")}
          </Button>
        </div>
        {available && (
          <p className="text-xs text-muted">
            {t("La disponibilidad se comprueba de nuevo al crear el bot.")}
          </p>
        )}
        {message && (
          <p role="status" className="break-words text-sm">
            {message}
          </p>
        )}
        {error && (
          <p role="alert" className="break-words text-sm text-danger">
            {error}
          </p>
        )}
        <p className="text-xs text-muted">
          {t(
            "El token se guarda cifrado en tu servidor. La verificación y el acceso a tus canales se completan en segundo plano.",
          )}
        </p>
      </div>
    </SettingsSection>
  );
}
