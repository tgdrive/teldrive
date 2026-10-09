import { Link, useNavigate, useRouterState } from "@tanstack/react-router"
import { Button, Input } from "@tw-material/react"
import debounce from "lodash.debounce"
import { memo, useEffect, useMemo, useRef, useState } from "react"
import IconDrive from "~icons/basil/google-drive-outline"
import IconSearch from "~icons/bi/search"
import IconClose from "~icons/ic/round-close"
import IconFilter from "~icons/mdi/filter-outline"
import { NewMenu } from "./menus/new-menu"
import { ProfileDropDown } from "./menus/profile"
import { SearchMenu } from "./menus/search/search"
import { ThemeToggle } from "./menus/theme-toggle"

const SearchBar = memo(function SearchBar() {
  const navigate = useNavigate()
  const location = useRouterState({ select: (state) => state.location })
  const routeQuery = (location.search as { query?: string }).query || ""
  const [query, setQuery] = useState(routeQuery)
  const [isOpen, setIsOpen] = useState(false)
  const inputRef = useRef<HTMLInputElement>(null)
  const triggerRef = useRef<HTMLButtonElement>(null)
  const search = useMemo(() => debounce((value: string) => {
    void navigate({ to: "/$view", params: { view: "search" }, search: { query: value.trim().replace(/\s+/g, " "), searchType: "text" }, replace: true })
  }, 400), [navigate])

  useEffect(() => { setQuery(routeQuery); search.cancel() }, [routeQuery, location.pathname, search])
  useEffect(() => () => search.cancel(), [search])
  useEffect(() => {
    function handleKeyDown(event: KeyboardEvent) {
      if ((event.ctrlKey || event.metaKey) && event.key.toLowerCase() === "k") {
        event.preventDefault()
        inputRef.current?.focus()
        inputRef.current?.select()
      }
    }
    window.addEventListener("keydown", handleKeyDown)
    return () => window.removeEventListener("keydown", handleKeyDown)
  }, [])

  function clearSearch() {
    search.cancel()
    setQuery("")
    if (location.pathname === "/search") void navigate({ to: "/$view", params: { view: "my-drive" }, search: { path: "/" } })
    inputRef.current?.focus()
  }

  return (
    <>
      <Input ref={inputRef} variant="flat" placeholder="Buscar en Teldrive" enterKeyHint="search" autoComplete="off" aria-label="Buscar archivos" value={query}
        onValueChange={(value) => { setQuery(value); search.cancel(); if (value.trim()) search(value); else clearSearch() }}
        onKeyDown={(event) => { if (event.key === "Enter" && query.trim()) search.flush(); if (event.key === "Escape") clearSearch() }}
        startContent={<IconSearch className="size-5 shrink-0 text-on-surface-variant" />}
        endContent={<div className="flex items-center gap-1">
          {query && <Button isIconOnly variant="text" size="sm" aria-label="Borrar búsqueda" onPress={clearSearch}><IconClose className="size-5" /></Button>}
          <Button isIconOnly variant="text" size="sm" ref={triggerRef} aria-label="Opciones de búsqueda" title="Opciones de búsqueda" onPress={() => setIsOpen((value) => !value)}><IconFilter className="size-5" /></Button>
        </div>}
        classNames={{ base: "flex-1 min-w-0 max-w-3xl", inputWrapper: "rounded-full min-h-12 bg-surface-container-high group-data-[focus=true]:bg-surface shadow-none", input: "px-2 text-base" }} />
      {isOpen && <SearchMenu isOpen={isOpen} setIsOpen={setIsOpen} triggerRef={triggerRef} />}
    </>
  )
})

export default memo(function Header({ auth }: { auth?: boolean }) {
  return (
    <header className="shrink-0 flex flex-wrap items-center gap-3 px-4 py-3 md:h-20">
      <Link to="/$view" params={{ view: "my-drive" }} search={{ path: "/" }} className="flex items-center gap-2 md:hidden">
        <IconDrive className="size-7 text-primary" /><span className="text-xl">Teldrive</span>
      </Link>
      {!auth && <span className="hidden md:block text-xl">Teldrive</span>}
      <div className="ml-auto flex items-center gap-2 md:order-last">
        <ThemeToggle />{auth && <ProfileDropDown />}
      </div>
      {auth && <div className="flex w-full md:w-auto md:flex-1 min-w-0 items-center gap-2"><div className="md:hidden"><NewMenu /></div><SearchBar /></div>}
    </header>
  )
})
