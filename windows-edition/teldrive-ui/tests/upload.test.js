import { beforeEach, expect, test } from "bun:test"
import { useFileUploadStore } from "../src/utils/stores/upload"

beforeEach(() => {
  useFileUploadStore.setState({ filesIds: [], fileMap: {}, currentFileId: "", uploadOpen: false })
})

test("drag and drop accepts files when the browser has no directory API", async () => {
  const file = new File(["sample"], "report.txt", { type: "text/plain" })
  await useFileUploadStore.getState().actions.handleDragDrop([{ kind: "file", getAsFile: () => file }])
  const state = useFileUploadStore.getState()
  expect(state.filesIds).toHaveLength(1)
  expect(state.fileMap[state.filesIds[0]].file.name).toBe("report.txt")
  expect(state.uploadOpen).toBe(true)
})

test("null directory entries fall back to normal files and skip text drops", async () => {
  const file = new File(["image"], "photo.png")
  await useFileUploadStore.getState().actions.handleDragDrop([
    { kind: "string", getAsFile: () => null },
    { kind: "file", webkitGetAsEntry: () => null, getAsFile: () => file },
  ])
  expect(useFileUploadStore.getState().filesIds).toHaveLength(1)
})

test("folders retain their nested relative paths", async () => {
  const file = new File(["sample"], "notes.txt")
  const fileEntry = { isFile: true, isDirectory: false, name: "notes.txt", file: (resolve) => resolve(file) }
  let read = false
  const folder = { isDirectory: true, isFile: false, name: "Project", createReader: () => ({ readEntries: (resolve) => { resolve(read ? [] : [fileEntry]); read = true } }) }
  await useFileUploadStore.getState().actions.handleDragDrop([{ kind: "file", webkitGetAsEntry: () => folder }])
  const state = useFileUploadStore.getState()
  expect(state.filesIds).toHaveLength(2)
  expect(Object.values(state.fileMap).find((entry) => !entry.isFolder).relativePath).toBe("Project/notes.txt")
})
