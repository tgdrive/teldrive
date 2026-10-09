import { NewMenu } from "@/components/menus/new-menu"
import { StorageSummary } from "@/components/storage-summary"
import { Link, useLocation, useParams } from "@tanstack/react-router"
import clsx from "clsx"
import { memo } from "react"
import IconDrive from "~icons/basil/google-drive-outline"
import IconShare from "~icons/fluent/share-24-regular"
import IconStorage from "~icons/ic/outline-cloud"
import IconTrash from "~icons/ic/outline-delete"
import IconSpam from "~icons/ic/outline-report"
import IconSettings from "~icons/ic/outline-settings"
import IconRecent from "~icons/mdi/recent"

export const categories = [
  { id: "my-drive", name: "Mi unidad", icon: IconDrive },
  { id: "recent", name: "Recientes", icon: IconRecent },
  { id: "shared", name: "Mis enlaces", icon: IconShare },
  { id: "spam", name: "Spam", icon: IconSpam },
  { id: "trash", name: "Papelera", icon: IconTrash },
  { id: "storage", name: "Almacenamiento", icon: IconStorage },
] as const

export const SideNav = memo(function SideNav() {
  const params = useParams({ strict: false }) as { view?: string }
  const location = useLocation()
  return (
    <aside className="drive-sidebar shrink-0 w-full md:w-60 h-16 md:h-full md:pt-5 md:overflow-y-auto">
      <div className="hidden md:flex items-center gap-3 px-7 h-12 mb-5">
        <IconDrive className="size-8 text-primary" />
        <span className="text-2xl tracking-tight">Teldrive</span>
      </div>
      <div className="hidden md:block px-4 mb-6"><NewMenu /></div>
      <nav aria-label="Navegación principal" className="h-full md:h-auto">
        <ul className="flex h-full md:flex-col justify-evenly md:justify-start gap-1 md:px-3">
          {categories.map(({ id, name, icon: Icon }) => {
            const isActive = id === "storage" ? location.pathname === "/storage" : params.view === id
            return (
              <li key={id} className="flex-1 md:flex-none min-w-0">
                <Link to={id === "storage" ? "/storage" : "/$view"} params={{ view: id }} search={id === "my-drive" ? { path: "/" } : {}}
                  aria-current={isActive ? "page" : undefined} preload="intent"
                  className={clsx("flex flex-col md:flex-row items-center gap-1 md:gap-4 rounded-full px-2 md:px-5 py-2.5 text-xs md:text-sm transition-colors", isActive ? "bg-secondary-container text-on-secondary-container font-semibold" : "text-on-surface-variant hover:bg-on-surface/5")}>
                  <Icon className="size-5 shrink-0" /><span className="truncate max-w-full">{name}</span>
                </Link>
                {id === "storage" && <div className="hidden md:block px-2 pt-4 pb-2"><StorageSummary compact /></div>}
              </li>
            )
          })}
        </ul>
      </nav>
      <div className="hidden md:block mx-5 mt-6 pt-4 border-t border-outline-variant/50">
        <Link to="/settings/$tabId" params={{ tabId: "general" }} className="flex items-center gap-4 px-3 py-2 text-sm text-on-surface-variant rounded-full hover:bg-on-surface/5"><IconSettings className="size-5" />Configuración</Link>
      </div>
    </aside>
  )
})
