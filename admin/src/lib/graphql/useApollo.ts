import { ApolloClient, HttpLink, InMemoryCache, NormalizedCacheObject } from "@apollo/client";
import { setContext } from "@apollo/client/link/context";

import { getToken } from "lib/auth";

let apolloClient: ApolloClient<NormalizedCacheObject>;

function createApolloClient() {
  const httpLink = new HttpLink({ uri: "/api/graphql/" });

  const authLink = setContext(async (_, { headers }) => {
    const token = await getToken();
    return {
      headers: {
        ...headers,
        ...(token ? { Authorization: `Bearer ${token}` } : {}),
      },
    };
  });

  return new ApolloClient({
    ssrMode: typeof window === "undefined",
    link: authLink.concat(httpLink),
    cache: new InMemoryCache({
      typePolicies: {
        Query: {
          fields: {
            interceptedRequests: {
              merge(_, incoming) {
                return incoming;
              },
            },
          },
        },
        ProjectSettings: {
          merge: true,
        },
      },
    }),
  });
}

export function useApollo() {
  const _apolloClient = apolloClient ?? createApolloClient();

  // For SSG and SSR always create a new Apollo Client
  if (typeof window === "undefined") return _apolloClient;
  // Create the Apollo Client once in the client
  if (!apolloClient) apolloClient = _apolloClient;

  return _apolloClient;
}
