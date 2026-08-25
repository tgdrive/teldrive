import { expect, test, type APIRequestContext, type BrowserContext } from "@playwright/test";

const baseURL = process.env.TELDRIVE_UI_BASE_URL;
const ownerToken = process.env.TELDRIVE_UI_TEST_ACCESS_TOKEN;
const adminToken = process.env.TELDRIVE_UI_TEST_ADMIN_ACCESS_TOKEN;
const aliceToken = process.env.TELDRIVE_UI_TEST_ALICE_ACCESS_TOKEN;
const bobToken = process.env.TELDRIVE_UI_TEST_BOB_ACCESS_TOKEN;
const charlieToken = process.env.TELDRIVE_UI_TEST_CHARLIE_ACCESS_TOKEN;
const disabledToken = process.env.TELDRIVE_UI_TEST_DISABLED_ACCESS_TOKEN;

const readRootId = "70000000-0000-4000-8000-000000000001";
const readChildId = "70000000-0000-4000-8000-000000000002";
const editRootId = "70000000-0000-4000-8000-000000000003";
const editChildId = "70000000-0000-4000-8000-000000000004";
const privateRootId = "70000000-0000-4000-8000-000000000005";

function requireHarness() {
  test.skip(
    !baseURL ||
      !ownerToken ||
      !adminToken ||
      !aliceToken ||
      !bobToken ||
      !charlieToken ||
      !disabledToken,
    "requires scripts/test-ui.sh",
  );
}

async function authenticate(context: BrowserContext, token: string) {
  if (!baseURL) throw new Error("TELDRIVE_UI_BASE_URL is required");
  await context.clearCookies();
  await context.addCookies([{ name: "teldrive_access", value: token, url: baseURL }]);
}

function bearer(token: string) {
  return { Authorization: `Bearer ${token}` };
}

function mutationHeaders(token: string) {
  return { ...bearer(token), "Idempotency-Key": crypto.randomUUID() };
}

async function getMe(request: APIRequestContext, token: string) {
  return request.get("/api/v1/me", { headers: bearer(token) });
}

test.describe("real backend access control", () => {
  test.describe.configure({ mode: "serial" });

  test("database roles drive admin capabilities and UI visibility immediately", async ({
    context,
    page,
    request,
    isMobile,
  }) => {
    requireHarness();
    test.skip(isMobile, "role-management acceptance only needs one browser project");

    const ownerMe = await getMe(request, ownerToken!);
    expect(ownerMe.ok()).toBe(true);
    expect(await ownerMe.json()).toMatchObject({ userId: 1001, role: "owner" });

    const adminMe = await getMe(request, adminToken!);
    expect(adminMe.ok()).toBe(true);
    expect(await adminMe.json()).toMatchObject({ userId: 1002, role: "admin" });

    const disabledMe = await getMe(request, disabledToken!);
    expect(disabledMe.status()).toBe(401);

    const bobAdminList = await request.get("/api/v1/admin/users", { headers: bearer(bobToken!) });
    expect(bobAdminList.status()).toBe(403);

    await authenticate(context, ownerToken!);
    await page.goto("/settings/users");
    await expect(page.getByRole("heading", { name: "Users & roles" })).toBeVisible();
    await expect(page.getByText("Alice Example", { exact: true })).toBeVisible();
    await expect(page.getByText("Bob Example", { exact: true })).toBeVisible();

    await authenticate(context, bobToken!);
    await page.goto("/files");
    await expect(page.getByRole("link", { name: "Tasks", exact: true })).toHaveCount(0);

    const promote = await request.patch("/api/v1/admin/users/1004", {
      headers: bearer(ownerToken!),
      data: { role: "admin" },
    });
    expect(promote.ok()).toBe(true);

    const promotedMe = await getMe(request, bobToken!);
    expect(promotedMe.ok()).toBe(true);
    const promoted = (await promotedMe.json()) as { role: string; capabilities: string[] };
    expect(promoted.role).toBe("admin");
    expect(promoted.capabilities).toContain("system.manageUsers");

    const demote = await request.patch("/api/v1/admin/users/1004", {
      headers: bearer(ownerToken!),
      data: { role: "user" },
    });
    expect(demote.ok()).toBe(true);
  });

  test("internal grants enforce read, edit, expiry, revocation, and subtree isolation", async ({
    context,
    page,
    request,
    isMobile,
  }) => {
    requireHarness();
    test.skip(isMobile, "shared-with-me acceptance only needs one browser project");

    const sharedResponse = await request.get("/api/v1/shared-with-me", { headers: bearer(bobToken!) });
    expect(sharedResponse.ok()).toBe(true);
    const shared = (await sharedResponse.json()) as Array<{
      permission: "read" | "edit";
      file: { id: string; name: string };
    }>;
    expect(shared.map((entry) => [entry.file.name, entry.permission])).toEqual([
      ["Edit shared", "edit"],
      ["Read shared", "read"],
    ]);

    const readChildren = await request.get(`/api/v1/files?parentId=${readRootId}&limit=100`, {
      headers: bearer(bobToken!),
    });
    expect(readChildren.ok()).toBe(true);
    expect((await readChildren.json()).items).toEqual(
      expect.arrayContaining([expect.objectContaining({ id: readChildId, name: "readme.txt" })]),
    );

    const readRename = await request.patch(`/api/v1/files/${readChildId}`, {
      headers: bearer(bobToken!),
      data: { name: "must-not-rename.txt" },
    });
    expect(readRename.status()).toBe(403);

    const editRename = await request.patch(`/api/v1/files/${editChildId}`, {
      headers: bearer(bobToken!),
      data: { name: "edited-by-bob.txt" },
    });
    expect(editRename.ok()).toBe(true);

    const restoreName = await request.patch(`/api/v1/files/${editChildId}`, {
      headers: bearer(bobToken!),
      data: { name: "editable.txt" },
    });
    expect(restoreName.ok()).toBe(true);

    const reshare = await request.post(`/api/v1/files/${editRootId}/shares`, {
      headers: mutationHeaders(bobToken!),
      data: { permission: "read" },
    });
    expect([403, 404]).toContain(reshare.status());

    const privateLookup = await request.get(`/api/v1/files/${privateRootId}`, {
      headers: bearer(bobToken!),
    });
    expect([403, 404]).toContain(privateLookup.status());

    const charlieLookup = await request.get(`/api/v1/files/${readRootId}`, {
      headers: bearer(charlieToken!),
    });
    expect([403, 404]).toContain(charlieLookup.status());

    await authenticate(context, bobToken!);
    await page.goto("/shared-with-me");
    await expect(page.getByText("Read shared", { exact: true })).toBeVisible();
    await expect(page.getByText("Edit shared", { exact: true })).toBeVisible();
    await expect(page.getByText("Expired shared", { exact: true })).toHaveCount(0);
    await expect(page.getByText("Revoked shared", { exact: true })).toHaveCount(0);
  });

  test("public share permission controls anonymous mutation", async ({ request, isMobile }) => {
    requireHarness();
    test.skip(isMobile, "public permission acceptance only needs one browser project");

    const readFileResponse = await request.get(`/api/v1/files/${readChildId}`, {
      headers: bearer(aliceToken!),
    });
    expect(readFileResponse.ok()).toBe(true);
    const readGeneration = ((await readFileResponse.json()) as { generation: number }).generation;

    const editFileResponse = await request.get(`/api/v1/files/${editChildId}`, {
      headers: bearer(aliceToken!),
    });
    expect(editFileResponse.ok()).toBe(true);
    const editGeneration = ((await editFileResponse.json()) as { generation: number }).generation;

    const readShareResponse = await request.post(`/api/v1/files/${readRootId}/shares`, {
      headers: mutationHeaders(aliceToken!),
      data: { permission: "read" },
    });
    expect(readShareResponse.ok()).toBe(true);
    const readShare = (await readShareResponse.json()) as { id: string; token: string; permission: string };
    expect(readShare.permission).toBe("read");

    const editShareResponse = await request.post(`/api/v1/files/${editRootId}/shares`, {
      headers: mutationHeaders(aliceToken!),
      data: { permission: "edit" },
    });
    expect(editShareResponse.ok()).toBe(true);
    const editShare = (await editShareResponse.json()) as { id: string; token: string; permission: string };
    expect(editShare.permission).toBe("edit");

    const readMutation = await request.patch(
      `/api/v1/public/shares/${encodeURIComponent(readShare.token)}/files/${readChildId}`,
      {
        headers: { "If-Match": `"${readGeneration}"` },
        data: { name: "anonymous-read-cannot-edit.txt" },
      },
    );
    expect(readMutation.status()).toBe(403);

    const editMutation = await request.patch(
      `/api/v1/public/shares/${encodeURIComponent(editShare.token)}/files/${editChildId}`,
      { headers: { "If-Match": `"${editGeneration}"` }, data: { name: "anonymous-edit.txt" } },
    );
    expect(editMutation.ok()).toBe(true);

    const restoreName = await request.patch(
      `/api/v1/public/shares/${encodeURIComponent(editShare.token)}/files/${editChildId}`,
      { headers: { "If-Match": `"${editGeneration + 1}"` }, data: { name: "editable.txt" } },
    );
    expect(restoreName.ok()).toBe(true);

    expect(
      (
        await request.delete(`/api/v1/shares/${readShare.id}`, {
          headers: bearer(aliceToken!),
        })
      ).ok(),
    ).toBe(true);
    expect(
      (
        await request.delete(`/api/v1/shares/${editShare.id}`, {
          headers: bearer(aliceToken!),
        })
      ).ok(),
    ).toBe(true);
  });
});
