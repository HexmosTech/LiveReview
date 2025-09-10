import reducer, { initialToDoState, taskAdded } from './reducer';
import configureAppStore, { AppStore, StoreDispatch } from '../configureStore';
import { toggleTaskStatus } from './thunks';
import { selectAllTasks } from './selectors';

describe('test toggleTaskStatus thunk function', () => {
    let store: AppStore;

    beforeEach(() => {
        store = configureAppStore();
    });

    it('toggleTaskStatus should toggle the completed status of the task', async () => {
        const newTask = {
            id: '12345',
            name: 'my task',
            completed: false,
        };

    await (store.dispatch as StoreDispatch)(taskAdded(newTask));

    await (store.dispatch as StoreDispatch)(toggleTaskStatus(newTask.id));

        const allTasks = selectAllTasks(store.getState());

        expect(allTasks[0].completed).toBeTruthy();
    });
});
