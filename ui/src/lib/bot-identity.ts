import { t } from "@/lib/localization";

export type BotIdentity = { id: number; name?: string; username?: string };

export function botName(bot: BotIdentity): string {
  return bot.name?.trim() || t("Nombre pendiente");
}

export function botAlias(bot: BotIdentity): string {
  const username = bot.username?.trim().replace(/^@/, "");
  return username ? `@${username}` : t("Alias pendiente");
}

export function botIdentityLabel(bot: BotIdentity): string {
  return `${botName(bot)} · ${botAlias(bot)} · ID: ${bot.id}`;
}
