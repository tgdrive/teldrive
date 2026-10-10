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
      toast.success("Bot provisioning queued", {
        description:
          "Check Tasks for progress. Telegram may temporarily delay channel admin changes.",
      });
    },
    onError: (error) => {
      toast.error("Bot provisioning could not be queued", { description: userMessage(error) });
    },
  });
  const provisionAll = $api.useMutation("post", "/v1/bots/provision", {
    onSuccess: (result) => {
      setProvisionAllOpen(false);
      if (result.jobIds.length === 0) {
        toast.info("No bots to provision");
        return;
      }
      toast.success(
        `${result.jobIds.length} bot${result.jobIds.length === 1 ? "" : "s"} queued for provisioning`,
        {
          description: "Each bot has its own task. Check Tasks for progress.",
        },
      );
    },
    onError: (error) => {
      toast.error("Bots could not be queued for provisioning", { description: userMessage(error) });
    },
  });
  const remove = $api.useMutation("delete", "/v1/bots/{botId}", {
    onSuccess: () => {
      setDeleteBot(null);
      void refresh();
      toast.success("Telegram bot deleted");
    },
    onError: (error) => {
      toast.error("Telegram bot could not be deleted", { description: userMessage(error) });
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
      toast.error("Bots could not be queued", { description: userMessage(error) });
    } finally {
      setIsAddingBots(false);
    }
  };

  return (
    <div className="space-y-6">
      <SettingsPageHeader
        title="Telegram bots"
        description="Bot accounts used for parallel Telegram API throughput."
      />
      <SettingsSection
        title="Add bots"
        description="Paste one BotFather token per line. Each bot is verified and added to existing channels in its own background task."
      >
        <SettingsRow
          label="Bot tokens"
          description="Tokens are sent only to your Teldrive server and are never shown again."
        >
          <div className="flex flex-col items-stretch gap-2 sm:flex-row sm:items-end">
            <TextField className="min-w-0 flex-1">
              <Label className="sr-only">Bot tokens</Label>
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
              Add bots
            </Button>
          </div>
        </SettingsRow>
      </SettingsSection>
      <SettingsSection
        title="Configured bots"
        description="Healthy enabled bots are used automatically. After migration, use Provision to verify a bot and repair its channel access."
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
            Provision all bots
          </Button>
        }
      >
        {query.data.items.length ? (
          query.data.items.map((bot) => (
            <SettingsRow
              key={bot.id}
              label={`@${bot.username || `bot-${bot.id}`}`}
              description={`Added ${new Date(bot.createdAt).toLocaleString()}`}
            >
              <div className="flex items-center justify-end gap-2">
                <Chip color={bot.enabled ? "success" : "warning"} variant="tertiary">
                  {bot.enabled ? "Enabled" : "Disabled"}
                </Chip>
                <Button
                  size="sm"
                  variant="secondary"
                  aria-label={`Provision bot ${bot.username || bot.id}`}
                  isDisabled={provision.isPending || provisionAll.isPending || remove.isPending}
                  onPress={() =>
                    setProvisionBot({ id: bot.id, name: bot.username || `bot-${bot.id}` })
                  }
                >
                  Provision
                </Button>
                <Button
                  isIconOnly
                  size="sm"
                  variant="ghost"
                  aria-label={`Delete bot ${bot.username || bot.id}`}
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
          <div className="px-5 py-8 text-sm text-muted">No Telegram bots are configured.</div>
        )}
      </SettingsSection>
      <ConfirmDialog
        open={provisionAllOpen}
        onOpenChange={(open) => {
          if (!provisionAll.isPending) setProvisionAllOpen(open);
        }}
        title="Provision all Telegram bots?"
        message="Verify every configured bot and repair its admin access to your registered Telegram channels using your logged-in account. Each bot runs in its own background task, including bots not shown on this page. Track progress in Tasks."
        confirmLabel="Provision all bots"
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
        title="Provision Telegram bot?"
        message={`Verify “${provisionBot?.name ?? ""}” and add it as an admin to your registered Telegram channels. This runs in the background using your logged-in Telegram account. Track progress in Tasks.`}
        confirmLabel="Provision bot"
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
        title="Delete Telegram bot?"
        message={`Uploads using other bots or your user session will continue after “${deleteBot?.name ?? ""}” is removed.`}
        confirmLabel="Delete bot"
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
