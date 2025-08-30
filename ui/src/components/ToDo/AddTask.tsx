import { taskAdded } from '@store/ToDo/reducer';
import React, { useState } from 'react';
import TaskListIcon from './assets/list-check-32.svg';
import { useAppDispatch } from '@store/configureStore';

export const AddTask = () => {
    const dispatch = useAppDispatch();

    const [taskName, setTaskName] = useState<string>('');

    const handleSumbit = () => {
        if (!taskName) {
            return;
        }

        dispatch(
            taskAdded({
                id: performance.now().toString(), // just use a fake id
                name: taskName,
                completed: false,
            })
        );

        setTaskName(''); // Reset the value of the input
    };

    const handleKeyDown = (event: React.KeyboardEvent) => {
        if (event.key === 'Enter') {
            handleSumbit(); // Call the same function as the button click
        }
    };

    return (
        <div className="mt-8 mb-2 flex items-center">
            <img className=" w-6 h-6" src={TaskListIcon} />
            <div className="flex-grow ml-2">
                <input
                    className="block bg-white border border-slate-300 rounded-md py-1 px-2 shadow-sm focus:outline-none focus:border-sky-500 focus:ring-sky-500 focus:ring-1 sm:text-sm w-full"
                    placeholder="Enter the name of New Task"
                    type="text"
                    name="new task"
                    value={taskName}
                    onKeyDown={handleKeyDown} // Listen for key down events
                    onChange={(e) => setTaskName(e.target.value)}
                />
            </div>
            <div>
                <button
                    className="p-1 bg-sky-500 hover:bg-sky-700 border border-slate-300 rounded-md text-sm text-white ml-2 disabled:pointer-events-none disabled:opacity-50"
                    disabled={!taskName}
                    onClick={handleSumbit}
                >
                    Add New Task
                </button>
            </div>
        </div>
    );
};
