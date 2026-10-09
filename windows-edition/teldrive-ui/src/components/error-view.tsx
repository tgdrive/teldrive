export const ErrorView = ({ message }: { message: string }) => {
  return (
    <div className="m-auto mt-16 flex rounded-large max-w-md min-h-48 flex-col justify-center items-center bg-surface gap-4 px-4">
      <h1 className="text-2xl font-bold text-on-surface">Error</h1>
      <p className="text-lg text-on-surface capitalize">{message}</p>
    </div>
  );
};
