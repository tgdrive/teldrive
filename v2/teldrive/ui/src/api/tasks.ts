import type { QueryClient } from "@tanstack/react-query";
import { $api, fetchClient } from "./client";

export const taskApi = $api;
export { fetchClient };

export async function invalidateTaskQueries(queryClient: QueryClient, taskID?: string) {
  const invalidations = [
    queryClient.invalidateQueries({ queryKey: ["get", "/v1/jobs"] }),
    queryClient.invalidateQueries({ queryKey: ["get", "/v1/jobs/statistics"] }),
    queryClient.invalidateQueries({ queryKey: ["get", "/v1/jobs/queues"] }),
  ];

  if (taskID) {
    invalidations.push(
      queryClient.invalidateQueries({
        queryKey: ["get", "/v1/jobs/{jobId}", { params: { path: { jobId: taskID } } }],
      }),
    );
  }

  await Promise.all(invalidations);
}
