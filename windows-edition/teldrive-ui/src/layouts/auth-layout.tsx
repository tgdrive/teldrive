import { Outlet } from "@tanstack/react-router";
import clsx from "clsx";
import { memo } from "react";

import Header from "@/components/header";
import { SideNav } from "@/components/navs/side-nav";
import { TopLoader } from "@/components/top-progress";
import { scrollbarClasses } from "@/utils/classes";

export const AuthLayout = memo(() => {
  return (
    <div className="drive-shell flex flex-col-reverse md:flex-row h-dvh overflow-hidden bg-surface-container text-on-surface">
      <SideNav />
      <div className="relative flex flex-1 min-w-0 flex-col overflow-x-hidden">
        <Header auth />
        <main
          className={clsx(
            "drive-main w-auto min-h-0 flex-1 overflow-y-auto overflow-x-hidden p-4 md:p-6 md:mr-4 md:mb-4 bg-surface rounded-2xl",
            scrollbarClasses,
          )}
        >
          <TopLoader />
          <Outlet />
        </main>
      </div>
    </div>
  );
});
