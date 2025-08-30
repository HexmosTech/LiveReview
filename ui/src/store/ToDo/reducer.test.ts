import reducer, { initialToDoState, taskAdded } from './reducer';

describe('Redux Store - test ToDo reducer', () => {
    it('should contain correct initial state', () => {
        const initialState = reducer(initialToDoState, {} as any);
        expect(initialState).toMatchObject(initialToDoState);
    });

    it('should handle taskAdded action', () => {
        const newTask = {
            id: '12345',
            name: 'my task',
            completed: false,
        };

        const newState = reducer(initialToDoState, taskAdded(newTask));

        expect(newState.tasks.ids).toContain(newTask.id);
        expect(newState.tasks.byId[newTask.id]).toMatchObject(newTask);
    });
});
