import { useEffect } from 'react';
import { useRouter } from 'next/router';

const Page = () => {
  const router = useRouter();

  useEffect(() => {
    router.replace('/mass-actions/firmware');
  }, [router]);

  return null;
};

export default Page;
