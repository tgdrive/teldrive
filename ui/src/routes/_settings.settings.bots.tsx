import { Button, Chip, Label, Spinner, TextArea, TextField } from "@heroui/react";
import { createFileRoute } from "@tanstack/react-router";
import { useState } from "react";
import { toast } from "sonner";
import TrashIcon from "~icons/gravity-ui/trash-bin";
import { $api } from "@/api/client";
import { userMessage } from "@/api/errors";
import { ConfirmDialog } from "@/components/dialogs/confirm-dialog";
import { SettingsPageHeader, SettingsRow, SettingsSection } from "@/components/settings-layout";
import { newIdempotencyKey } from "@/features/shared/idempotency";
import { getQueryClient } from "@/lib/queryClient";
import { useAppLocale } from "@/lib/locale";
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
  const { t } = useAppLocale();
  const formatDateTime = useDateTimeFormatter();
  const query = $api.useSuspenseQuery(
    "get",
    "/v1/bots",
    { params: { query: { limit: 200 } } },
    { staleTime: 20_000 },
  );
  const create = $api.useMutation("post", "/v1/bots");
  const provision = $api.useMutation("post", "/v1/bots/{botId}/provision", {
    onSuccess: () => {
      setProvisionBot(null);
      toast.success(t("bots.provisionQueued"), {
        description: t("bots.provisionQueuedDesc"),
      });
    },
    onError: (error) => {
      toast.error(t("bots.provisionFailed"), { description: userMessage(error) });
    },
  });
  const provisionAll = $api.useMutation("post", "/v1/bots/provision", {
    onSuccess: (result) => {
      setProvisionAllOpen(false);
      if (result.jobIds.length === 0) {
        toast.info(t("bots.noBots"));
        return;
      }
      toast.success(
        t(result.jobIds.length === 1 ? "bots.queuedOne" : "bots.queuedMany", {
          count: result.jobIds.length,
        }),
        {
          description: t("bots.queuedDesc"),
        },
      );
    },
    onError: (error) => {
      toast.error(t("bots.queueAllFailed"), { description: userMessage(error) });
    },
  });
  const remove = $api.useMutation("delete", "/v1/bots/{botId}", {
    onSuccess: () => {
      setDeleteBot(null);
      void refresh();
      toast.success(t("bots.deleted"));
    },
    onError: (error) => {
      toast.error(t("bots.deleteFailed"), { description: userMessage(error) });
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
      toast.error(t("bots.addFailed"), { description: userMessage(error) });
    } finally {
      setIsAddingBots(false);
    }
  };

  return (
    <div className="space-y-6">
      <SettingsPageHeader title={t("bots.title")} description={t("bots.desc")} />
      <SettingsSection title={t("bots.add")} description={t("bots.addDesc")}>
        <SettingsRow label={t("bots.tokens")} description={t("bots.tokensDesc")}>
          <div className="flex flex-col items-stretch gap-2 sm:flex-row sm:items-end">
            <TextField className="min-w-0 flex-1">
              <Label className="sr-only">{t("bots.tokens")}</Label>
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
              {t("bots.addBtn")}
            </Button>
          </div>
        </SettingsRow>
      </SettingsSection>
      <SettingsSection
        title={t("bots.configured")}
        description={t("bots.configuredDesc")}
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
            {t("bots.provisionAll")}
          </Button>
        }
      >
        {query.data.items.length ? (
          query.data.items.map((bot) => (
            <SettingsRow
              key={bot.id}
              label={`@${bot.username || `bot-${bot.id}`}`}
              description={t("bots.addedAt", {
                date: formatDateTime.format(new Date(bot.createdAt)),
              })}
            >
              <div className="flex items-center justify-end gap-2">
                <Chip color={bot.enabled ? "success" : "warning"} variant="tertiary">
                  {t(bot.enabled ? "bots.enabled" : "bots.disabled")}
                </Chip>
                <Button
                  size="sm"
                  variant="secondary"
                  aria-label={t("bots.provisionAria", { name: bot.username || bot.id })}
                  isDisabled={provision.isPending || provisionAll.isPending || remove.isPending}
                  onPress={() =>
                    setProvisionBot({ id: bot.id, name: bot.username || `bot-${bot.id}` })
                  }
                >
                  {t("bots.provision")}
                </Button>
                <Button
                  isIconOnly
                  size="sm"
                  variant="ghost"
                  aria-label={t("bots.deleteAria", { name: bot.username || bot.id })}
                  isDisabled={remove.isPending && deleteBot?.id === bot.id}
                  onPress={() =>
                    setDeleteBot({ id: bot.id, name: bot.username || `bot-${bot.id}` })
                  }
                >
                  <TrashIcon className="size-4" />
                </Button>
              </div>
            </SettingsRow>
          ))
        ) : (
          <div className="px-5 py-8 text-sm text-muted">{t("bots.empty")}</div>
        )}
      </SettingsSection>
      <ConfirmDialog
        open={provisionAllOpen}
        onOpenChange={(open) => {
          if (!provisionAll.isPending) setProvisionAllOpen(open);
        }}
        title={t("bots.allTitle")}
        message={t("bots.allMsg")}
        confirmLabel={t("bots.allConfirm")}
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
        title={t("bots.oneTitle")}
        message={t("bots.oneMsg", { name: provisionBot?.name ?? "" })}
        confirmLabel={t("bots.oneConfirm")}
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
        title={t("bots.delTitle")}
        message={t("bots.delMsg", { name: deleteBot?.name ?? "" })}
        confirmLabel={t("bots.delConfirm")}
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
