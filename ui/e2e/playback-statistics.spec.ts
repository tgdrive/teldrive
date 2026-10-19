import { expect, test } from "@playwright/test";
import { alphaId, installFileApi } from "./file-api.fixture";

for (const kind of ["audio", "video"]) {
  test(`${kind} developer statistics show live bot identity and stop polling when hidden`, async ({page}) => {
    await installFileApi(page);
    const file = {id:alphaId,name:`prueba.${kind === "audio" ? "mp3" : "mp4"}`,kind:"file",status:"active",generation:1,mimeType:`${kind}/${kind === "audio" ? "mpeg" : "mp4"}`,size:1048576,encryption:false,modTime:"2026-10-19T00:00:00Z",createdAt:"2026-10-19T00:00:00Z",updatedAt:"2026-10-19T00:00:00Z"};
    let requests = 0;
    let speed = 1048576;
    await page.route("**/api/v1/**", async route => {
      const path = new URL(route.request().url()).pathname;
      if (path === "/api/v1/files") return route.fulfill({json:{items:[file]}});
      if (path.endsWith("/playback/statistics")) {
        requests++;
        return route.fulfill({json:{scope:"file",fileId:alphaId,name:file.name,mimeType:file.mimeType,size:file.size,encrypted:false,sampledAt:new Date().toISOString(),activeStreams:1,bytes:2097152,bytesPerSecond:speed,accounts:[{id:56789,name:"Biblioteca principal",alias:"biblioteca_bot",kind:"bot",bytes:2097152,bytesPerSecond:speed,requests:4,errors:1,activeRequests:1,latencyMs:45.5,channelId:123,messageId:27,offset:1048576,lastSeen:new Date().toISOString()}]}});
      }
      if (path.endsWith("/playback")) return route.fulfill({status:201,json:{ticket:"a".repeat(64),expiresAt:"2099-01-01T00:00:00Z",conversionAvailable:false}});
      if (path === "/api/v1/playback/tracks") return route.fulfill({json:{available:true,tracks:[{index:0,kind,codec:kind === "audio" ? "mp3" : "h264",language:"spa",title:"Principal",default:true,bitmap:false}]}});
      if (path.includes("/content")) return route.fulfill({contentType:file.mimeType,body:Buffer.alloc(16)});
      return route.fallback();
    });
    await page.goto("/files");
    await page.getByRole("row",{name:new RegExp(file.name)}).dblclick();
    await page.getByRole("button",{name:"Desarrollador · estadísticas",exact:true}).click();
    const stats = page.getByRole("region",{name:"Estadísticas de reproducción"});
    await expect(stats).toContainText("Biblioteca principal");
    await expect(stats).toContainText("@biblioteca_bot");
    await expect(stats).toContainText("ID 56789");
    await expect(stats).toContainText("1.00 MiB/s");
    await expect(stats).toContainText("45.5 ms");
    speed = 0;
    await expect(stats).toContainText("0.00 MiB/s");
    if (kind === "video") {
      const fullscreen=page.getByRole("button",{name:"Pantalla completa",exact:true}).last();
      await expect(fullscreen).toBeVisible();
      if(test.info().project.name === "chromium") {
        await fullscreen.click();
        await expect.poll(() => page.evaluate(() => !!document.fullscreenElement)).toBe(true);
        await page.evaluate(() => document.exitFullscreen());
        await expect.poll(() => page.evaluate(() => !!document.fullscreenElement)).toBe(false);
      }
    }
    await page.getByRole("button",{name:"Desarrollador · estadísticas",exact:true}).click();
    await expect(stats).toHaveCount(0);
    const stopped = requests;
    await page.waitForTimeout(1300);
    expect(requests).toBe(stopped);
    expect(await page.locator("body").innerText()).not.toContain("a".repeat(64));
  });
}
