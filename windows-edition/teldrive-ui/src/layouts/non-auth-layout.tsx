import { memo } from "react";
import { Outlet } from "@tanstack/react-router";

import Header from "@/components/header";

export const NonAuthLayout = memo(() => {
  return (
    <div className="flex flex-col-reverse md:flex-row h-dvh overflow-hidden">
      <div className="relative flex flex-1 flex-col overflow-x-hidden">
        <Header />
        <main className="container">
          <Outlet />
        </main>
      </div>
    </div>
  );
});
