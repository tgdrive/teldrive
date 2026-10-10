import { expect, test } from "@playwright/test";
import { installFileApi } from "./file-api.fixture";

test("spam removes owned files from drive and restores them", async ({page}) => {
  await installFileApi(page);
  await page.goto("/files");
  await page.getByRole("row",{name:/alpha\.txt/}).locator('[data-slot="checkbox-control"]').click();
  await page.getByRole("button",{name:"Marcar elementos seleccionados como spam"}).click();
  await expect(page.getByRole("row",{name:/alpha\.txt/})).toBeHidden();
  await page.goto("/spam");
  await expect(page.getByText("alpha.txt",{exact:true})).toBeVisible();
  await page.getByRole("button",{name:"No es spam",exact:true}).click();
  await expect(page.getByText("alpha.txt",{exact:true})).toBeHidden();
  await page.goto("/files");
  await expect(page.getByRole("row",{name:/alpha\.txt/})).toBeVisible();
});

test("password protected public audio uses one playback session and converts without leaking password", async ({page}) => {
  await installFileApi(page);
  const id="22222222-2222-4222-8222-222222222222";
  const file={id,name:"prueba.wav",kind:"file",mimeType:"audio/wav",size:16044,status:"active",generation:1,encryption:false,createdAt:"2026-07-01T00:00:00Z",updatedAt:"2026-07-01T00:00:00Z",modTime:"2026-07-01T00:00:00Z"};
  let sessions=0;
  const streams:string[]=[];
  await page.route("**/api/v1/public/shares/**",async route => {
    const request=route.request();
    if (request.headers()["x-share-password"]!=="secreto") return route.fulfill({status:401,json:{error:{code:"share_password_required",message:"Password required"}}});
    if (request.url().endsWith("/playback")){sessions++;return route.fulfill({status:201,json:{ticket:"b".repeat(64),expiresAt:"2099-01-01T00:00:00Z",conversionAvailable:true}});}
    return route.fulfill({json:{file,permission:"read"}});
  });
  await page.route("**/api/v1/playback?**",async route => {
    streams.push(route.request().url());
    return route.fulfill({contentType:"audio/wav",body:wave()});
  });
  await page.goto("/share/prueba");
  await page.getByLabel("Contraseña",{exact:true}).fill("secreto");
  await page.getByRole("button",{name:"Abrir elemento compartido"}).click();
  await page.getByRole("row",{name:/prueba.wav/}).locator('[data-slot="checkbox-control"]').click();
  await page.getByRole("button",{name:"Vista previa",exact:true}).click();
  const dialog=page.getByRole("dialog");
  await expect(dialog.locator("audio")).toBeVisible();
  await dialog.getByRole("button",{name:"Reproducción compatible",exact:true}).click();
  await expect(dialog.locator("audio")).toHaveAttribute("src",/mode=audio/);
  await dialog.getByLabel("Comenzar en segundos").fill("12");
  await dialog.getByRole("button",{name:"Reiniciar desde esta posición"}).click();
  await expect(dialog.locator("audio")).toHaveAttribute("src",/start=12/);
  expect(sessions).toBe(1);
  expect(streams.every(url=>!url.includes("secreto"))).toBeTruthy();
  await dialog.getByRole("button",{name:"Versión original"}).click();
  await expect(dialog.locator("audio")).toHaveAttribute("src",/mode=original/);
});

function wave(){
 const data=Buffer.alloc(16044);data.write("RIFF");data.writeUInt32LE(16036,4);data.write("WAVEfmt ",8);data.writeUInt32LE(16,16);data.writeUInt16LE(1,20);data.writeUInt16LE(1,22);data.writeUInt32LE(8000,24);data.writeUInt32LE(16000,28);data.writeUInt16LE(2,32);data.writeUInt16LE(16,34);data.write("data",36);data.writeUInt32LE(16000,40);return data;
}
