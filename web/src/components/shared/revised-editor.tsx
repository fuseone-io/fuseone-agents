import { Fragment, useState, type ReactNode } from "react";
import type { UseQueryResult } from "@tanstack/react-query";
import {
  PropertiesSheet,
  PropertiesSheetBody,
} from "@/components/shared/properties-sheet";
import { ErrorState, LoadingRows } from "@/components/shared/states";
import type { FormRevision, Revised } from "@/lib/api/revision";

/**
 * An edit sheet seeded from the resource as it is stored now.
 *
 * Read by name as it opens, never from the row it was opened from: that row
 * is as old as the list, and a save over it would answer a revision the person
 * never saw. Until the read answers, the sheet says it is reading; if it
 * cannot read, it says so and offers to try again.
 */
export function RevisedEditor<T>({
  read,
  title,
  description,
  onClose,
  children,
}: {
  read: UseQueryResult<Revised<T>>;
  title: string;
  description: string;
  onClose: () => void;
  children: (body: T, revision: FormRevision) => ReactNode;
}) {
  // Counted, because a revision is named by what is stored: a state that came
  // back has the revision it had, and keying by it alone would keep the draft
  // a reload was asked to replace.
  const [reloads, setReloads] = useState(0);
  // Only a read made as this editor opened seeds it: a copy cached by an
  // earlier opening may be older than what is stored now.
  if (read.data && read.isFetchedAfterMount && !read.isError) {
    return (
      <Fragment key={`${read.data.etag}:${reloads}`}>
        {children(read.data.body, {
          etag: read.data.etag,
          reload: () => void read.refetch().then(() => setReloads((n) => n + 1)),
          reloading: read.isFetching,
        })}
      </Fragment>
    );
  }
  return (
    <PropertiesSheet
      open
      onOpenChange={(open) => !open && onClose()}
      title={title}
      description={description}
    >
      <PropertiesSheetBody>
        {read.error ? (
          <ErrorState error={read.error} onRetry={() => void read.refetch()} />
        ) : (
          <LoadingRows rows={4} />
        )}
      </PropertiesSheetBody>
    </PropertiesSheet>
  );
}
