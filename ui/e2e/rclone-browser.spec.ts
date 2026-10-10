import { expect, test } from "@playwright/test";
import { installFileApi } from "./file-api.fixture";

test.beforeEach(async ({ page }) => {
  await installFileApi(page);
  await page.addInitScript(() => {
    const listeners = new Set<(event: {data: unknown}) => void>();
    const requests: {method: string; params: Record<string, unknown>}[] = [];
    const state = {configured: true, encrypted: false, locked: false, driverInstalled: true, version: "test"};
    let name = "documento.txt";
    const jobs: {id: string; label: string; mount: boolean; status: string; log: string}[] = [];
    Object.assign(window, {nativeRequests: requests});
    Object.assign(window.chrome ?? (window.chrome = {}), {webview: {
      addEventListener: (_: string, listener: (event: {data: unknown}) => void) => listeners.add(listener),
      removeEventListener: (_: string, listener: (event: {data: unknown}) => void) => listeners.delete(listener),
      postMessage: ({id, method, params}: {id: string; method: string; params: Record<string, unknown>}) => {
        requests.push({method, params}); let result: unknown = true;
        if (method === "rclone.info") result = {...state};
        if (method === "rclone.browse") result = name ? [{Name:name, Path:name, Size:20, IsDir:false, ModTime:"2026-10-09T00:00:00Z"}] : [];
        if (method === "rclone.rename") name = String(params.target).split("/").pop()!;
        if (method === "rclone.trash") name = "";
        if (method === "rclone.protect") { state.encrypted = true; result = {...state}; }
        if (method === "rclone.lock") { state.locked = true; result = {...state}; }
        if (method === "rclone.unlock") { state.locked = false; result = {...state}; }
        if (method === "rclone.jobs") result = jobs.map(job => ({...job}));
        if (["rclone.upload", "rclone.upload-folder", "rclone.download", "rclone.mount"].includes(method)) { result = crypto.randomUUID(); jobs.push({id:String(result), label:method === "rclone.mount" ? "Unidad T:" : "Transferencia", mount:method === "rclone.mount", status:"running", log:"10% transferido"}); }
        if (method === "rclone.stop") jobs.find(job => job.id === params.id)!.status = "cancelled";
        if (method === "rclone.retry") jobs.find(job => job.id === params.id)!.status = "running";
        queueMicrotask(() => { for (const listener of listeners) listener({data:{id,result}}); });
      },
    }});
  });
});

test("native explorer renames and removes the selected remote file", async ({ page }) => {
  await page.goto("/settings/rclone");
  await page.getByRole("radio", {name:"Seleccionar documento.txt"}).check();
  await page.getByRole("button", {name:"Renombrar", exact:true}).click();
  const dialog = page.getByRole("dialog", {name:"Renombrar en rclone"});
  await dialog.getByLabel("Nombre en rclone").fill("nuevo.txt");
  await dialog.getByRole("button", {name:"Guardar", exact:true}).click();
  await expect(page.getByRole("radio", {name:"Seleccionar nuevo.txt"})).toBeVisible();
  await page.getByRole("radio", {name:"Seleccionar nuevo.txt"}).check();
  await page.getByRole("button", {name:"Eliminar", exact:true}).click();
  await page.getByRole("alertdialog").getByRole("button", {name:"Eliminar", exact:true}).click();
  await expect(page.getByText("No hay elementos que mostrar.")).toBeVisible();
});

test("configuration protection locks the explorer until unlocked", async ({ page }) => {
  await page.goto("/settings/rclone");
  await page.getByLabel("Contraseña de rclone", {exact:true}).fill("contraseña-de-prueba");
  await page.getByLabel("Repetir contraseña de rclone").fill("contraseña-de-prueba");
  await page.getByRole("button", {name:"Cifrar configuración", exact:true}).click();
  await page.getByRole("button", {name:"Bloquear y detener rclone", exact:true}).click();
  await expect(page.getByRole("radio", {name:"Seleccionar documento.txt"})).toBeHidden();
  await page.getByLabel("Contraseña de rclone", {exact:true}).fill("contraseña-de-prueba");
  await page.getByRole("button", {name:"Desbloquear rclone", exact:true}).click();
  await expect(page.getByRole("radio", {name:"Seleccionar documento.txt"})).toBeVisible();
  await expect(page.getByLabel("Contraseña de rclone", {exact:true})).toBeHidden();
});

test("transfer controls start, stop and retry a native job", async ({ page }) => {
  await page.goto("/settings/rclone");
  await page.getByRole("button", {name:"Subir carpeta local", exact:true}).click();
  await expect(page.getByText("10% transferido")).toBeAttached();
  await page.getByRole("button", {name:"Detener", exact:true}).click();
  await expect(page.getByText("Detenida", {exact:true})).toBeVisible();
  await page.getByRole("button", {name:"Reintentar", exact:true}).click();
  await expect(page.getByText("En curso", {exact:true})).toBeVisible();
});
