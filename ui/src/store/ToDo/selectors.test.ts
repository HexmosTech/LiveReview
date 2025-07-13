import reducer, { initialToDoState, taskAdded } from './reducer';
import { selectAllTasks } from './selectors';

describe('test selectAllTasks', () => {
    it('should return all tasks', () => {
        const newTask = {
            id: '12345',
            name: 'my task',
            completed: false,
        };

        const state = reducer(
            {
                tasks: {
                    byId: {
                        [newTask.id]: newTask,
                    },
                    ids: [newTask.id],
                },
            },
            {} as any
        );

        const allTasks = selectAllTasks({ ToDo: state });

        expect(allTasks.length).toBe(1);
        expect(allTasks[0]).toMatchObject(newTask);
    });
});
