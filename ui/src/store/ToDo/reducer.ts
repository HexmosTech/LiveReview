import {
    createSlice,
    // createSelector,
    PayloadAction,
    // createAsyncThunk
} from '@reduxjs/toolkit';

// import { RootState, StoreDispatch, StoreGetState } from '../configureStore';

type Task = {
    id: string;
    name: string;
    completed: boolean;
};

export type ToDoState = {
    /**
     * tasks data
     */
    tasks: {
        byId: {
            [key: string]: Task;
        };
        ids: string[];
    };
};

export const initialToDoState: ToDoState = {
    tasks: {
        byId: {},
        ids: [],
    },
};

const slice = createSlice({
    name: 'ToDo',
    initialState: initialToDoState,
    reducers: {
        taskAdded: (state, action: PayloadAction<Task>) => {
            const { id } = action.payload;
            state.tasks.byId[id] = action.payload;
            state.tasks.ids.push(id);
        },
        taskToggled: (state, action: PayloadAction<string>) => {
            const id = action.payload;
            state.tasks.byId[id].completed = !state.tasks.byId[id].completed;
        },
    },
});

const { reducer } = slice;

export const { taskAdded, taskToggled } = slice.actions;

export default reducer;
