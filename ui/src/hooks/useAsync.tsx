import { useState, useCallback } from 'react';

type AsyncFunction<T> = (...args: any[]) => Promise<T>;

/**
 * Custom hook for managing asynchronous operations in a React component.
 *
 * @template T - Specifies the type of value returned by the asynchronous function.
 * @param {AsyncFunction<T>} asyncFunction - The asynchronous function to be executed.
 * @returns {Object} An object with the following properties:
 * - `isLoading` (boolean): Indicates if the asynchronous operation is currently in progress.
 * - `error` (Error | null): Holds any error encountered during the execution of the async function.
 * - `execute` (Function): A function used to invoke the async function with any provided arguments.
 *
 * @example
 * const { isLoading, error, execute } = useAsync(fetchData);
 *
 * useEffect(() => {
 *   execute();
 * }, [execute]);
 */
export function useAsync<T>(asyncFunction: AsyncFunction<T>) {
    const [isLoading, setIsLoading] = useState(false);

    const [error, setError] = useState<Error | null>(null);

    const execute = useCallback(
        async (...args: any[]) => {
            setIsLoading(true);
            setError(null);

            try {
                const result = await asyncFunction(...args);
                return result;
            } catch (err) {
                setError(err as Error);
                throw err;
            } finally {
                setIsLoading(false);
            }
        },
        [asyncFunction]
    );

    return { isLoading, error, execute };
}
