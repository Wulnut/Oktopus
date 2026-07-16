import { createContext, useContext, useEffect, useReducer, useRef } from 'react';
import PropTypes from 'prop-types';
import { useRouter } from 'next/router';

function parseJwt(token) {
  if (!token || typeof token !== 'string') {
    return null;
  }
  const parts = token.split('.');
  if (parts.length !== 3) {
    return null;
  }
  try {
    const base64 = parts[1].replace(/-/g, '+').replace(/_/g, '/');
    return JSON.parse(window.atob(base64));
  } catch {
    return null;
  }
}

const HANDLERS = {
  INITIALIZE: 'INITIALIZE',
  SIGN_IN: 'SIGN_IN',
  SIGN_OUT: 'SIGN_OUT'
};

const initialState = {
  isAuthenticated: false,
  isLoading: true,
  user: null
};

const handlers = {
  [HANDLERS.INITIALIZE]: (state, action) => {
    const user = action.payload;

    return {
      ...state,
      ...(
        // if payload (user) is provided, then is authenticated
        user
          ? ({
            isAuthenticated: true,
            isLoading: false,
            user
          })
          : ({
            isLoading: false
          })
      )
    };
  },
  [HANDLERS.SIGN_IN]: (state, action) => {
    const user = action.payload;

    return {
      ...state,
      isAuthenticated: true,
      user
    };
  },
  [HANDLERS.SIGN_OUT]: (state) => {
    return {
      ...state,
      isAuthenticated: false,
      user: null
    };
  }
};

const reducer = (state, action) => (
  handlers[action.type] ? handlers[action.type](state, action) : state
);

// The role of this context is to propagate authentication state through the App tree.

export const AuthContext = createContext({ undefined });

export const AuthProvider = (props) => {
  const { children } = props;
  const [state, dispatch] = useReducer(reducer, initialState);
  const initialized = useRef(false);
  const router = useRouter();

  const initialize = async () => {
    // Prevent from calling twice in development mode with React.StrictMode enabled
    if (initialized.current) {
      return;
    }

    initialized.current = true;

    let isAuthenticated = false;

    try {
      isAuthenticated = window.sessionStorage.getItem('authenticated') === 'true';
    } catch (err) {
      console.error(err);
    }

    if (isAuthenticated) {
      const email = window.sessionStorage.getItem('email') || '';
      const tenantId = window.sessionStorage.getItem('tenantId') || '';
      const tenantSlug = window.sessionStorage.getItem('tenantSlug') || '';
      const level = parseInt(window.sessionStorage.getItem('level') || '0', 10);
      const token = window.localStorage.getItem('token') || '';
      const user = {
        avatar: '/assets/avatars/default-avatar.png',
        name: email,
        email: email,
        token,
        tenantId,
        tenantSlug,
        level,
      };

      dispatch({
        type: HANDLERS.INITIALIZE,
        payload: user
      });
    } else {
      dispatch({
        type: HANDLERS.INITIALIZE
      });
    }
  };

  useEffect(
    () => {
      initialize();
    },

    []
  );

  const skip = () => {
    try {
      window.sessionStorage.setItem('authenticated', 'true');
    } catch (err) {
      console.error(err);
    }

    const email = window.sessionStorage.getItem('email') || '';
    const user = {
      avatar: '/assets/avatars/default-avatar.png',
      name: email,
      email: email,
    };

    dispatch({
      type: HANDLERS.SIGN_IN,
      payload: user
    });
  };

  const signIn = async (email, password) => {

    var myHeaders = new Headers();
    myHeaders.append("Content-Type", "application/json");

    var raw = JSON.stringify({
      "email": email,
      "password": password
    });

    var requestOptions = {
      method: 'PUT',
      headers: myHeaders,
      body: raw,
      redirect: 'follow'
    };

    let result = await fetch(`${process.env.NEXT_PUBLIC_REST_ENDPOINT || ""}/api/auth/login`, requestOptions)

    if (result.status != 200) {
      throw new Error('Please check your email and password');
    }

    const token = await result.json()

    const claims = parseJwt(token);

    try {
      window.sessionStorage.setItem('authenticated', 'true');
      window.sessionStorage.setItem('email', email);
      window.sessionStorage.setItem('tenantSlug', claims?.tenant_slug || '');
      window.sessionStorage.setItem('tenantId', claims?.tenant_id || '');
      window.sessionStorage.setItem('level', String(claims?.level ?? 0));
    } catch (err) {
      console.error(err);
    }

    localStorage.setItem("token", token)

    const user = {
      avatar: '/assets/avatars/default-avatar.png',
      name: claims?.username || email,
      email: email,
      token,
      tenantId: claims?.tenant_id || '',
      tenantSlug: claims?.tenant_slug || '',
      level: claims?.level ?? 0,
    };

    dispatch({
      type: HANDLERS.SIGN_IN,
      payload: user
    });
  };

  const signUp = async (email, name, password) => {
    var myHeaders = new Headers();
    myHeaders.append("Content-Type", "application/json");

    var raw = JSON.stringify({
      "email": email,
      "password": password,
      "name": name
    });

    var requestOptions = {
      method: 'POST',
      headers: myHeaders,
      body: raw,
      redirect: 'follow'
    };

    let result = await fetch(`${process.env.NEXT_PUBLIC_REST_ENDPOINT || ""}/api/auth/admin/register`, requestOptions)

    if (result.status == 200) {
      router.push("/auth/login")
    }else{
      const content = await result.json()
      throw new Error(content);
    }

  };

  const signOut = () => {
    router.push("/auth/login")
    localStorage.removeItem("token")
    window.sessionStorage.removeItem('tenantSlug');
    window.sessionStorage.removeItem('tenantId');
    window.sessionStorage.removeItem('level');
    window.sessionStorage.removeItem('activeTenantSlug');
    dispatch({
      type: HANDLERS.SIGN_OUT
    });
  };

  return (
    <AuthContext.Provider
      value={{
        ...state,
        skip,
        signIn,
        signUp,
        signOut,
      }}
    >
      {children}
    </AuthContext.Provider>
  );
};

AuthProvider.propTypes = {
  children: PropTypes.node
};

export const AuthConsumer = AuthContext.Consumer;

export const useAuthContext = () => useContext(AuthContext);
