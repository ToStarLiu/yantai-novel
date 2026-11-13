CREATE FUNCTION pg_temp.novel_verify_counts() RETURNS jsonb LANGUAGE plpgsql AS $$
DECLARE item record; amount bigint; result jsonb := '{}'::jsonb;
BEGIN
 FOR item IN SELECT tablename FROM pg_tables
   WHERE schemaname='public' AND tablename LIKE 'wb_%'
     AND tablename NOT IN ('wb_sessions','wb_presence') ORDER BY tablename
 LOOP
   EXECUTE format('SELECT count(*) FROM public.%I',item.tablename) INTO amount;
   result := jsonb_set(result,ARRAY[item.tablename],to_jsonb(amount));
 END LOOP;
 RETURN result;
END $$;
SELECT pg_temp.novel_verify_counts();
