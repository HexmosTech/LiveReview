import React from 'react';
import { TaskList } from './TaskList';
import { AddTask } from './AddTask';

export const ToDoList = () => {
    return (
        <div className=" max-w-md mx-auto">
            <AddTask />
            <TaskList />
        </div>
    );
};
