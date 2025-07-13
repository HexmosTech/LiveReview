import { createSelector } from '@reduxjs/toolkit';
import { RootState } from '../configureStore';

export const selectAllTasks = createSelector(
    (state: RootState) => state.ToDo.tasks,
    (tasks) => {
        const { byId, ids } = tasks;
        return ids.map((id) => byId[id]);
    }
);

export const selectCountOfCompletedTasks = createSelector(
    (state: RootState) => state.ToDo.tasks,
    (tasks) => {
        const { byId, ids } = tasks;
        return ids
            .filter((id) => byId[id].completed === true)
            .map((id) => byId[id]).length;
    }
);
