// Deprecated - will be removed
import { createSelector } from '@reduxjs/toolkit';

// These selectors are deprecated and will be removed
// They are kept for backward compatibility with tests
export const selectAllTasks = createSelector(
    (state: Record<string, any>) => state.ToDo?.tasks || { byId: {}, ids: [] },
    (tasks) => {
        const { byId, ids } = tasks;
        return ids.map((id: string) => byId[id]);
    }
);

export const selectCountOfCompletedTasks = createSelector(
    (state: Record<string, any>) => state.ToDo?.tasks || { byId: {}, ids: [] },
    (tasks) => {
        const { byId, ids } = tasks;
        return ids
            .filter((id: string) => byId[id]?.completed === true)
            .map((id: string) => byId[id]).length;
    }
);
