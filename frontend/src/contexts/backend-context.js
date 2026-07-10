import { createContext, useContext, useCallback } from 'react';
import { useRouter } from 'next/router';
import { useAlertContext } from './error-context';
import { useTenant } from './tenant-context';

export const BackendContext = createContext({ undefined });

export const BackendProvider = (props) => {
    const { children } = props;

    const { setAlert } = useAlertContext();
    const { apiPrefix } = useTenant();

    const router = useRouter();

    const httpRequest = useCallback(async (path, method, body, headers, encoding) => {
        var requestOptions = {
            method: method,
            redirect: 'follow',
        };

        if (body) {
            requestOptions.body = body;
        }

        if (headers) {
            requestOptions.headers = headers;
        } else {
            // Read token fresh per request (not cached at init)
            const h = new Headers();
            h.append("Content-Type", "application/json");
            const token = localStorage.getItem("token");
            if (token) {
                h.append("Authorization", token);
            }
            requestOptions.headers = h;
        }

        // Abort the request after a timeout so loading states always resolve.
        // Without this, a hung backend (controller/adapter down) leaves the
        // UI spinner spinning forever because fetch() never settles.
        const timeoutMs = 30000;
        const controller = new AbortController();
        const timeoutId = setTimeout(() => controller.abort(), timeoutMs);
        requestOptions.signal = controller.signal;

        let response;
        try {
            response = await fetch(`${process.env.NEXT_PUBLIC_REST_ENDPOINT || ""}${path}`, requestOptions);
        } catch (err) {
            clearTimeout(timeoutId);
            if (err.name === 'AbortError') {
                setAlert({
                    severity: "error",
                    message: "Request timed out. The backend may be unreachable.",
                });
                return { status: 0, result: null };
            }
            throw err;
        }
        clearTimeout(timeoutId);
        if (response.status === 401) {
            router.push("/auth/login");
            return {status: response.status, result: null};
        }
        if (!response.ok) {
            if (response.status === 429) {
                setAlert({
                    severity: "warning",
                    message: "Too many requests. Please wait a moment and try again.",
                });
                return {status : response.status, result: null};
            }
            const data = await response.text();
            setAlert({
                severity: "error",
                message: `${data}`,
            });
            return {status : response.status, result: null};
        }
        if (response.status === 204) {
            return {status: response.status, result: null};
        }
        if (encoding) {
            if (encoding == "text") {
                const data = await response.text();
                return {status: response.status, result: data};
            } else if (encoding == "blob") {
                const data = await response.blob();
                return {status: response.status, result: data};
            } else if (encoding == "json") {
                const data = await response.json();
                return {status: response.status, result: data};
            } else {
                return {status: response.status, result: response};
            }
        }
        const data = await response.json();
        return {status: response.status, result: data};
    }, [router, setAlert]);

    return (
        <BackendContext.Provider
        value={{
            httpRequest,
            setAlert,
            apiPrefix,
        }}
        >
        {children}
        </BackendContext.Provider>
    );
};

export const BackendConsumer = BackendContext.Consumer;

export const useBackendContext = () => useContext(BackendContext);
