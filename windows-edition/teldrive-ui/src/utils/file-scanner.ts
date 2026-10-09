export const scanEntries = async (entry: FileSystemEntry, path = ""): Promise<File[]> => {
  try {
    if (entry.isFile) {
      return new Promise((resolve, reject) => {
        (entry as FileSystemFileEntry).file(
          (file: File) => {
            Object.defineProperty(file, "webkitRelativePath", {
              value: path ? `${path}/${file.name}` : file.name,
              writable: false,
            });
            resolve([file]);
          },
          (err) => reject(err),
        );
      });
    }
    if (entry.isDirectory) {
      const directoryReader = (entry as FileSystemDirectoryEntry).createReader();
      return new Promise((resolve, reject) => {
        const readAllEntries = async () => {
          const entries: FileSystemEntry[] = [];
          let keepReading = true;
          while (keepReading) {
            const batch = await new Promise<FileSystemEntry[]>((res, rej) => {
              directoryReader.readEntries(res, rej);
            });
            if (batch.length > 0) {
              entries.push(...batch);
            } else {
              keepReading = false;
            }
          }
          const filePromises = entries.map((e) =>
            scanEntries(e, path ? `${path}/${entry.name}` : entry.name),
          );
          const files = await Promise.all(filePromises);
          resolve(files.flat());
        };
        readAllEntries().catch(reject);
      });
    }
  } catch (error) {
    console.error("Entry scan failed:", error);
  }
  return [];
};
