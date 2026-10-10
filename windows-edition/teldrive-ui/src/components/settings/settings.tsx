import { Outlet, getRouteApi } from "@tanstack/react-router";
import { Button } from "@tw-material/react";
import clsx from "clsx";
import { memo, useEffect, useState } from "react";
import CodiconAccount from "~icons/codicon/account";
import CodiconSettings from "~icons/codicon/settings";
import FluentDarkTheme20Filled from "~icons/fluent/dark-theme-20-filled";
import IcOutlineInfo from "~icons/ic/outline-info";
import IcRoundDesktopWindows from "~icons/ic/round-desktop-windows";

import { ForwardLink } from "@/components/forward-link";
import { motion } from "framer-motion";

const Tabs = [
  {
    id: "general",
    label: "General",
    icon: CodiconSettings,
  },
  {
    id: "appearance",
    label: "Apariencia",
    icon: FluentDarkTheme20Filled,
  },
  {
    id: "account",
    label: "Cuenta",
    icon: CodiconAccount,
  },
  { id: "server", label: "Servidor y CLI", icon: CodiconSettings },
  { id: "rclone", label: "rclone", icon: IcRoundDesktopWindows },
  { id: "info", label: "Información", icon: IcOutlineInfo },
];

const fileRoute = getRouteApi("/_authed/settings/$tabId");

export const Settings = memo(() => {
  const params = fileRoute.useParams();
  const [desktop, setDesktop] = useState(false);
  useEffect(() => {
    let active = true;
    void fetch("/desktop/api/status").then(async (response) => {
      if (response.ok) { const data = await response.json(); if (active) setDesktop(data.desktop === true); }
    }).catch(() => {});
    return () => { active = false; };
  }, []);
  const tabs = desktop ? [...Tabs.slice(0, -1), { id: "desktop", label: "Programa Windows", icon: IcRoundDesktopWindows }, Tabs[Tabs.length - 1]] : Tabs;

  return (
    <div className="bg-surface container size-full rounded-xl flex flex-col md:flex-row max-w-5xl gap-6 p-4">
      <div className="flex flex-col gap-4 w-full md:w-1/4">
        <h1 className="text-2xl font-semibold pt-2 px-2">Configuración</h1>
        <nav className="flex flex-row md:flex-col gap-1 overflow-x-auto no-scrollbar">
          {tabs.map((tab) => tab.id === "desktop" ? <Button key={tab.id} variant="text" className="h-14 flex-shrink-0 text-on-surface-variant" startContent={<IcRoundDesktopWindows />} onPress={() => { window.location.href = "/desktop"; }}>Programa Windows</Button> : (
            <div key={tab.id} className="relative w-auto md:w-full flex-shrink-0">
              <Button
                as={ForwardLink}
                variant="text"
                to="/settings/$tabId"
                disableRipple
                data-selected={params.tabId === tab.id}
                replace
                params={{ tabId: tab.id }}
                className={clsx(
                  "text-inherit h-14 w-full !justify-center md:!justify-start !px-4 z-1",
                  "data-[hover=true]:text-on-surface text-on-surface-variant",
                  "data-[selected=true]:text-on-surface data-[hover=true]:bg-transparent",
                  "[&>span>svg]:data-[hover=true]:scale-110 ",
                  "[&>span>svg]:data-[selected=true]:scale-110",
                  "flex-col md:flex-row items-center",
                  "gap-1 md:gap-2",
                )}
                startContent={<tab.icon className="text-xl" />}
              >
                <span className="text-xs md:text-base">{tab.label}</span>
              </Button>
              {params.tabId === tab.id && (
                <motion.span
                  className="absolute rounded-full inset-x-1 bottom-0 h-1 md:inset-y-1 md:left-0 md:right-0 md:h-auto z-0 bg-secondary-container text-on-secondary-container"
                  layoutId="pill"
                  transition={{
                    type: "spring",
                    bounce: 0.1,
                    duration: 0.4,
                  }}
                />
              )}
            </div>
          ))}
        </nav>
      </div>
      <div className="flex-1 overflow-hidden">
        <Outlet />
      </div>
    </div>
  );
});
