import { t } from "@/lib/localization";
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
import { getQueryClient } from "@/lib/queryClient";
import TrashIcon from "~icons/gravity-ui/trash-bin";
import { useDateTimeFormatter } from "@/lib/format";

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
  const [provisionBot, setProvisionBot] = useState<{ id: number; name: string } | null>(null);
  const [provisionAllOpen, setProvisionAllOpen] = useState(false);

  const _formatDateTime = useDateTimeFormatter();
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
  const provision = $api.useMutation("post", "/v1/bots/{botId}/provision", {
    onSuccess: () => {
      setProvisionBot(null);
      toast.success(t("Bot provisioning queued"), {
        description: t(
          "Check Tasks for progress. Telegram may temporarily delay channel admin changes.",
        ),
      });
    },
    onError: (error) => {
      toast.error(t("Bot provisioning could not be queued"), { description: userMessage(error) });
    },
  });
  const provisionAll = $api.useMutation("post", "/v1/bots/provision", {
    onSuccess: (result) => {
      setProvisionAllOpen(false);
      if (result.jobIds.length === 0) {
        toast.info(t("No bots to provision"));
        return;
      }
      toast.success(
        t(result.jobIds.length === 1 ? "bots.queuedOne" : "bots.queuedMany", {
          count: result.jobIds.length,
        }),
        {
          description: t("Each bot has its own task. Check Tasks for progress."),
        },
      );
    },
    onError: (error) => {
      toast.error(t("Bots could not be queued for provisioning"), {
        description: userMessage(error),
      });
    },
  });
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
          t(result.bots.length === 1 ? "bots.verifiedOne" : "bots.verifiedMany", {
            count: result.bots.length,
          }),
        );
      }
      if (result.failedIndexes.length > 0) {
        toast.warning(
          t(result.failedIndexes.length === 1 ? "bots.invalidOne" : "bots.invalidMany", {
            count: result.failedIndexes.length,
          }),
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
          "Paste one BotFather token per line. Each bot is verified and added to existing channels in its own background task.",
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
          "Healthy enabled bots are used automatically. After migration, use Provision to verify a bot and repair its channel access.",
        )}
        actions={
          <Button
            size="sm"
            variant="secondary"
            isDisabled={
              !query.data.items.length ||
              provisionAll.isPending ||
              provision.isPending ||
              remove.isPending
            }
            isPending={provisionAll.isPending}
            onPress={() => setProvisionAllOpen(true)}
          >
            {t("Provision all bots")}
          </Button>
        }
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
                  size="sm"
                  variant="secondary"
                  aria-label={t("Provision bot {name}", { name: bot.username || bot.id })}
                  isDisabled={provision.isPending || provisionAll.isPending || remove.isPending}
                  onPress={() =>
                    setProvisionBot({ id: bot.id, name: bot.username || `bot-${bot.id}` })
                  }
                >
                  {t("Provision")}
                </Button>
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
        open={provisionAllOpen}
        onOpenChange={(open) => {
          if (!provisionAll.isPending) setProvisionAllOpen(open);
        }}
        title={t("Provision all Telegram bots?")}
        message={t(
          "Verify every configured bot and repair its admin access to your registered Telegram channels using your logged-in account. Each bot runs in its own background task, including bots not shown on this page. Track progress in Tasks.",
        )}
        confirmLabel={t("Provision all bots")}
        isDestructive={false}
        isPending={provisionAll.isPending}
        onConfirm={() => {
          if (!provisionAll.isPending) {
            provisionAll.mutate({ params: { header: { "Idempotency-Key": newIdempotencyKey() } } });
          }
        }}
      />
      <ConfirmDialog
        open={provisionBot !== null}
        onOpenChange={(open) => {
          if (!open && !provision.isPending) setProvisionBot(null);
        }}
        title={t("Provision Telegram bot?")}
        message={t(
          "Verify “{name}” and add it as an admin to your registered Telegram channels. This runs in the background using your logged-in Telegram account. Track progress in Tasks.",
          { name: provisionBot?.name ?? "" },
        )}
        confirmLabel={t("Provision bot")}
        isDestructive={false}
        isPending={provision.isPending}
        onConfirm={() => {
          if (provisionBot) {
            provision.mutate({
              params: {
                path: { botId: provisionBot.id },
                header: { "Idempotency-Key": newIdempotencyKey() },
              },
            });
          }
        }}
      />
      <ConfirmDialog
        open={deleteBot !== null}
        onOpenChange={(open) => {
          if (!open && !remove.isPending) setDeleteBot(null);
        }}
        title={t("¿Eliminar este bot de Telegram?")}
        message={t(
          "Uploads using other bots or your user session will continue after “{name}” is removed.",
          { name: deleteBot?.name ?? "" },
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
