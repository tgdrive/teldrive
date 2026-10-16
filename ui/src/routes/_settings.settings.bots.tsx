import { Button, Chip, Label, Spinner, TextArea, TextField } from "@heroui/react";
import { createFileRoute } from "@tanstack/react-router";
import { useState } from "react";
import { toast } from "sonner";
import { $api } from "@/api/client";
import { userMessage } from "@/api/errors";
import { ConfirmDialog } from "@/components/dialogs/confirm-dialog";
import { ManagedBotSettings } from "@/components/managed-bot-settings";
import { SettingsPageHeader, SettingsRow, SettingsSection } from "@/components/settings-layout";
import { newIdempotencyKey } from "@/features/shared/idempotency";
import { botAlias, botIdentityLabel, botName } from "@/lib/bot-identity";
import { t } from "@/lib/localization";
import { getQueryClient } from "@/lib/queryClient";
import TrashIcon from "~icons/gravity-ui/trash-bin";

export const Route = createFileRoute("/_settings/settings/bots")({
  component: BotsSettings,
  pendingComponent: () => (
    <div className="flex justify-center py-16">
      <Spinner size="lg" />
    </div>
  ),
});

function BotsSettings() {
  const [token, setToken] = useState("");
  const [isAddingBots, setIsAddingBots] = useState(false);
  const [deleteBot, setDeleteBot] = useState<{ id: number; name: string } | null>(null);
  const query = $api.useSuspenseQuery(
    "get",
    "/v1/bots",
    { params: { query: { limit: 200 } } },
    {
      staleTime: 20_000,
      refetchInterval: (query) =>
        query.state.data?.items.some((bot) => !bot.name || !bot.username) ? 15_000 : false,
    },
  );
  const bots = [...query.data.items].sort(
    (a, b) =>
      (a.name || a.username || String(a.id)).localeCompare(
        b.name || b.username || String(b.id),
        undefined,
        { sensitivity: "base", numeric: true },
      ) || a.id - b.id,
  );
  const create = $api.useMutation("post", "/v1/bots");
  const remove = $api.useMutation("delete", "/v1/bots/{botId}", {
    onSuccess: () => {
      setDeleteBot(null);
      void refresh();
      toast.success(t("Bot de Telegram eliminado"));
    },
    onError: (error) => {
      toast.error(t("No se pudo eliminar el bot de Telegram"), { description: userMessage(error) });
    },
  });
  const refresh = () =>
    getQueryClient().invalidateQueries({ queryKey: $api.queryOptions("get", "/v1/bots").queryKey });

  const handleAddBots = async () => {
    const tokens = [
      ...new Set(
        token
          .split(/\r?\n/)
          .map((value) => value.trim())
          .filter(Boolean),
      ),
    ];
    if (tokens.length === 0 || isAddingBots) return;
    setIsAddingBots(true);

    try {
      const result = await create.mutateAsync({
        params: { header: { "Idempotency-Key": newIdempotencyKey() } },
        body: { tokens },
      });
      const failed = new Set(result.failedIndexes);
      setToken(tokens.filter((_, index) => failed.has(index)).join("\n"));
      await refresh();
      if (result.bots.length > 0) {
        toast.success(
          `${result.bots.length} bot${result.bots.length === 1 ? "" : "s"} queued for verification`,
        );
      }
      if (result.failedIndexes.length > 0) {
        toast.warning(
          `${result.failedIndexes.length} token${result.failedIndexes.length === 1 ? "" : "s"} had an invalid format`,
        );
      }
    } catch (error) {
      toast.error(t("No se pudieron programar los bots"), { description: userMessage(error) });
    } finally {
      setIsAddingBots(false);
    }
  };

  return (
    <div className="space-y-6">
      <SettingsPageHeader
        title={t("Bots de Telegram")}
        description={t("Bots utilizados para transferir datos en paralelo a Telegram.")}
      />
      <ManagedBotSettings bots={bots} onCreated={refresh} />
      <SettingsSection
        title={t("Añadir bots")}
        description={t(
          "Pega una clave de BotFather por línea. Los bots se guardan al instante y los canales existentes se actualizan en segundo plano.",
        )}
      >
        <SettingsRow
          label={t("Claves de bots")}
          description={t(
            "Las claves se envían únicamente a tu servidor Teldrive y no se vuelven a mostrar.",
          )}
        >
          <div className="flex flex-col items-stretch gap-2 sm:flex-row sm:items-end">
            <TextField className="min-w-0 flex-1">
              <Label className="sr-only">{t("Claves de bots")}</Label>
              <TextArea
                value={token}
                onChange={(event) => setToken(event.currentTarget.value)}
                placeholder={"123456:ABC...\n789012:DEF..."}
                rows={5}
                className="h-32 min-h-32 max-h-32 resize-none overflow-y-auto font-mono"
              />
            </TextField>
            <Button
              className="shrink-0"
              onPress={handleAddBots}
              isDisabled={!token.trim() || isAddingBots}
              isPending={isAddingBots}
            >
              {t("Añadir bots")}
            </Button>
          </div>
        </SettingsRow>
      </SettingsSection>
      <SettingsSection
        title={t("Bots configurados")}
        description={t(
          "Los bots activos disponibles se utilizan automáticamente para las transferencias.",
        )}
      >
        {bots.length ? (
          bots.map((bot) => (
            <SettingsRow
              key={bot.id}
              label={botName(bot)}
              description={
                <>
                  <span className="block break-all">{botAlias(bot)}</span>
                  <span className="block font-mono tabular-nums">ID: {bot.id}</span>
                  <span className="mt-1 block">
                    {t("Added {0}", [new Date(bot.createdAt).toLocaleString()])}
                  </span>
                </>
              }
            >
              <div className="flex items-center justify-end gap-2">
                <Chip color={bot.enabled ? "success" : "warning"} variant="tertiary">
                  {bot.enabled ? t("Activado") : t("Desactivado")}
                </Chip>
                <Button
                  isIconOnly
                  size="sm"
                  variant="ghost"
                  aria-label={t("Eliminar bot {0}", [botIdentityLabel(bot)])}
                  isDisabled={remove.isPending && deleteBot?.id === bot.id}
                  onPress={() => setDeleteBot({ id: bot.id, name: botIdentityLabel(bot) })}
                >
                  <TrashIcon className="size-4" />
                </Button>
              </div>
            </SettingsRow>
          ))
        ) : (
          <div className="px-5 py-8 text-sm text-muted">
            {t("No Telegram bots are configured.")}
          </div>
        )}
      </SettingsSection>
      <ConfirmDialog
        open={deleteBot !== null}
        onOpenChange={(open) => {
          if (!open && !remove.isPending) setDeleteBot(null);
        }}
        title={t("¿Eliminar este bot de Telegram?")}
        message={t(
          "Uploads using other bots or your user session will continue after “{0}” is removed.",
          [deleteBot?.name ?? ""],
        )}
        confirmLabel={t("Eliminar bot")}
        isPending={remove.isPending}
        onConfirm={() => {
          if (deleteBot) {
            remove.mutate({ params: { path: { botId: deleteBot.id } } });
          }
        }}
      />
    </div>
  );
}
